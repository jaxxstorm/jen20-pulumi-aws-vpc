package vpc

import (
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/iam"

	"github.com/imdario/mergo"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/config"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/route53"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

// Vpc is the return type of the package
type Vpc struct {
	pulumi.ResourceState

	ID             pulumi.IDOutput      `pulumi:"ID"`
	Args           Args                 `pulumi:"Args"`
	Cidr           pulumi.StringOutput  `pulumi:"Cidr"`
	Arn            pulumi.StringOutput  `pulumi:"Arn"`
	Vpc            ec2.Vpc              `pulumi:"Vpc"`
	PublicSubnets  pulumi.IDArrayOutput `pulumi:"PublicSubnets"`
	PrivateSubnets pulumi.IDArrayOutput `pulumi:"PrivateSubnets"`
}

type Endpoints struct {
	S3       bool
	DynamoDB bool
}

// Args are the arguments passed to the resource
type Args struct {
	BaseCidr              string
	Description           string
	ZoneName              pulumi.String
	AvailabilityZoneNames pulumi.StringArray
	BaseTags              pulumi.StringMap
	Endpoints             Endpoints
}

func resourceTags(tags pulumi.StringMap, baseTags pulumi.StringMap) pulumi.StringMap {
	mergo.Merge(&tags, baseTags)
	return tags
}

// creates a new VPC
func NewVpc(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Vpc, error) {
	vpc := &Vpc{}

	// create the VPC
	awsVpc, err := ec2.NewVpc(ctx, fmt.Sprintf("%s-vpc", name), &ec2.VpcArgs{
		CidrBlock:          pulumi.String(args.BaseCidr),
		EnableDnsSupport:   pulumi.Bool(true),
		EnableDnsHostnames: pulumi.Bool(true),
		Tags: resourceTags(args.BaseTags, pulumi.StringMap{
			"Name": pulumi.Sprintf("%s VPC", args.Description),
		}),
	}, pulumi.Parent(vpc))
	if err != nil {
		return nil, err
	}

	// export some VPC outputs
	vpc.ID = awsVpc.ID()
	vpc.Cidr = awsVpc.CidrBlock
	vpc.Arn = awsVpc.Arn

	// add an internet gateway
	igw, err := ec2.NewInternetGateway(ctx, fmt.Sprintf("%s-igw", name), &ec2.InternetGatewayArgs{
		VpcId: awsVpc.ID(),
		Tags: resourceTags(args.BaseTags, pulumi.StringMap{
			"Name": pulumi.Sprintf("%s VPC Internet Gateway", args.Description),
		}),
	}, pulumi.Parent(awsVpc))

	/*
	 * if a zone name is specified, create a route53 zone
	 * and set some DHCP options for the VPC
	 */
	if args.ZoneName != "" {
		// creates the route53 private zone
		zone, err := route53.NewZone(ctx, fmt.Sprintf("%s-private-zone", name), &route53.ZoneArgs{
			Name:    args.ZoneName,
			Comment: pulumi.Sprintf("Private zone for %s. Managed by Pulumi", args.ZoneName),
			Vpcs: route53.ZoneVpcArray{
				&route53.ZoneVpcArgs{
					VpcId: awsVpc.ID(),
				},
			},
		}, pulumi.Parent(vpc))
		if err != nil {
			return nil, err
		}

		// creates the DHCP option set
		dhcpOptionSet, err := ec2.NewVpcDhcpOptions(ctx, fmt.Sprintf("%s-dhcp-options", name), &ec2.VpcDhcpOptionsArgs{
			DomainName: zone.Name,
			DomainNameServers: pulumi.StringArray{
				pulumi.String("AmazonProvidedDNS"),
			},
			Tags: resourceTags(args.BaseTags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s DHCP Options", args.Description),
			}),
		}, pulumi.Parent(awsVpc))
		if err != nil {
			return nil, err
		}
		_, err = ec2.NewVpcDhcpOptionsAssociation(ctx, fmt.Sprintf("%s-dhcp-options-assoc", name), &ec2.VpcDhcpOptionsAssociationArgs{
			VpcId:         awsVpc.ID(),
			DhcpOptionsId: dhcpOptionSet.ID(),
		}, pulumi.Parent(dhcpOptionSet))
		if err != nil {
			return nil, err
		}
	}

	// returns the subnets as calculated by the subnet distributor
	privateSubnets, publicSubnets, err := SubnetDistributor(args.BaseCidr, len(args.AvailabilityZoneNames))
	if err != nil {
		return nil, err
	}

	// for storing the private subnets
	var awsPrivateSubnets []ec2.Subnet
	var awsPrivateSubnetIDs []pulumi.IDOutput

	// loop over all the private subnets and create
	for index, subnet := range privateSubnets {
		pSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-private-%d", name, index+1), &ec2.SubnetArgs{
			VpcId:            awsVpc.ID(),
			CidrBlock:        pulumi.String(subnet),
			AvailabilityZone: args.AvailabilityZoneNames[index],
			Tags: resourceTags(args.BaseTags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s Private %d", args.Description, index),
			}),
		}, pulumi.Parent(awsVpc))

		// append to slice of private subnets for use later
		awsPrivateSubnets = append(awsPrivateSubnets, *pSubnet)
		awsPrivateSubnetIDs = append(awsPrivateSubnetIDs, pSubnet.ID())

		if err != nil {
			return nil, err
		}
	}

	// for storing the public subnets
	var awsPublicSubnets []ec2.Subnet
	var awsPublicSubnetIDs []pulumi.IDOutput

	// loop over all the private subnets and create
	for index, subnet := range publicSubnets {
		pSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-public-%d", name, index+1), &ec2.SubnetArgs{
			VpcId:               awsVpc.ID(),
			CidrBlock:           pulumi.String(subnet),
			MapPublicIpOnLaunch: pulumi.Bool(true),
			AvailabilityZone:    args.AvailabilityZoneNames[index],
			Tags: resourceTags(args.BaseTags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s Public %d", args.Description, index),
			}),
		}, pulumi.Parent(awsVpc))

		// append to a slice of public subnets for use later
		awsPublicSubnets = append(awsPublicSubnets, *pSubnet)
		awsPublicSubnetIDs = append(awsPublicSubnetIDs, pSubnet.ID())

		if err != nil {
			return nil, err
		}
	}

	// adopt the default route table and make it usable for public subnets
	publicRouteTable, err := ec2.NewDefaultRouteTable(ctx, fmt.Sprintf("%s-public-rt", name), &ec2.DefaultRouteTableArgs{
		DefaultRouteTableId: awsVpc.DefaultRouteTableId,
		Tags: resourceTags(args.BaseTags, pulumi.StringMap{
			"Name": pulumi.Sprintf("%s Public Route Table", args.Description),
		}),
	}, pulumi.Parent(awsVpc))
	if err != nil {
		return nil, err
	}

	// route all public subnets to internet gateway
	_, err = ec2.NewRoute(ctx, fmt.Sprintf("%s-route-public-sn-to-ig", name), &ec2.RouteArgs{
		RouteTableId:         publicRouteTable.ID(),
		DestinationCidrBlock: pulumi.String("0.0.0.0/0"),
		GatewayId:            igw.ID(),
	}, pulumi.Parent(publicRouteTable))
	if err != nil {
		return nil, err
	}

	for index, subnet := range awsPublicSubnets {
		_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("%s-public-rta-%d", name, index+1), &ec2.RouteTableAssociationArgs{
			SubnetId:     subnet.ID(),
			RouteTableId: publicRouteTable.ID(),
		}, pulumi.Parent(publicRouteTable))
		if err != nil {
			return nil, err
		}
	}

	// sets up the routing for private subnets via a NAT gateway
	for index, subnet := range awsPrivateSubnets {
		elasticIP, err := ec2.NewEip(ctx, fmt.Sprintf("%s-nat-%d", name, index+1), &ec2.EipArgs{
			Tags: resourceTags(args.BaseTags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s NAT Gateway EIP %d", args.Description, index),
			}),
		}, pulumi.Parent(&subnet))
		if err != nil {
			return nil, err
		}

		natGateway, err := ec2.NewNatGateway(ctx, fmt.Sprintf("%s-nat-gateway-%d", name, index+1), &ec2.NatGatewayArgs{
			AllocationId: elasticIP.ID(),
			SubnetId:     awsPublicSubnets[index].ID(),
			Tags: resourceTags(args.BaseTags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s NAT Gateway %d", args.Description, index),
			}),
		}, pulumi.Parent(&subnet))
		if err != nil {
			return nil, err
		}

		privateRouteTable, err := ec2.NewRouteTable(ctx, fmt.Sprintf("%s-private-rt-%d", name, index+1), &ec2.RouteTableArgs{
			VpcId: awsVpc.ID(),
			Tags: resourceTags(args.BaseTags, pulumi.StringMap{
				"Name": pulumi.Sprintf("%s Private Subnet RT %d", args.Description, index),
			}),
		}, pulumi.Parent(awsVpc))
		if err != nil {
			return nil, err
		}

		_, err = ec2.NewRoute(ctx, fmt.Sprintf("%s-route-private-sn-to-nat-%d", name, index+1), &ec2.RouteArgs{
			RouteTableId:         privateRouteTable.ID(),
			DestinationCidrBlock: pulumi.String("0.0.0.0/0"),
			NatGatewayId:         natGateway.ID(),
		}, pulumi.Parent(privateRouteTable))
		if err != nil {
			return nil, err
		}

		_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("%s-private-rta-%d", name, index+1), &ec2.RouteTableAssociationArgs{
			SubnetId:     subnet.ID(),
			RouteTableId: privateRouteTable.ID(),
		}, pulumi.Parent(privateRouteTable))
		if err != nil {
			return nil, err
		}
	}

	// set up endpoints
	if args.Endpoints.S3 {
		_, err = ec2.NewVpcEndpoint(ctx, fmt.Sprintf("%s-s3-endpoint", name), &ec2.VpcEndpointArgs{
			VpcId:       awsVpc.ID(),
			ServiceName: pulumi.String(fmt.Sprintf("com.amazonaws.%s.s3", config.GetRegion(ctx))),
		}, pulumi.Parent(awsVpc))
		if err != nil {
			return nil, err
		}
	}
	if args.Endpoints.DynamoDB {
		_, err = ec2.NewVpcEndpoint(ctx, fmt.Sprintf("%s-dynamodb-endpoint", name), &ec2.VpcEndpointArgs{
			VpcId:       awsVpc.ID(),
			ServiceName: pulumi.String(fmt.Sprintf("com.amazonaws.%s.dynamodb", config.GetRegion(ctx))),
		}, pulumi.Parent(awsVpc))
		if err != nil {
			return nil, err
		}
	}

	// Register component resource
	err = ctx.RegisterComponentResource("jen20:aws-vpc", name, vpc, opts...)
	if err != nil {
		return nil, err
	}

	ctx.RegisterResourceOutputs(vpc, pulumi.Map{
		"ID":             awsVpc.ID(),
		"Cidr":           awsVpc.CidrBlock,
		"Arn":            awsVpc.Arn,
		"PublicSubnets":  idOutputArrayToIDArrayOutput(awsPublicSubnetIDs),
		"PrivateSubnets": idOutputArrayToIDArrayOutput(awsPrivateSubnetIDs),
	})

	return vpc, nil
}

// Optionally enable cloudwatch logging
// FIXME: should be a method?
func (vpc Vpc) EnableFlowLoggingToCloudWatchLogs(ctx *pulumi.Context, name string, trafficType string) error {

	// IAM policy principal
	assumeRolePolicyJSON, err := json.Marshal(map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []interface{}{
			map[string]interface{}{
				"Action": "sts:AssumeRole",
				"Principal": map[string]interface{}{
					"Service": "vpc-flow-logs.amazonaws.com",
				},
				"Effect": "Allow",
			},
		},
	})
	if err != nil {
		return err
	}

	flowLogsIAMRole, err := iam.NewRole(ctx, fmt.Sprintf("%s-flow-logs-role", name), &iam.RoleArgs{
		Description:      pulumi.String(fmt.Sprintf("%s VPC Flow Logs", name)),
		AssumeRolePolicy: pulumi.String(assumeRolePolicyJSON),
	}, pulumi.Parent(&vpc))
	if err != nil {
		return err
	}

	flowlogLogGroup, err := cloudwatch.NewLogGroup(ctx, fmt.Sprintf("%s-vpc-flow-logs", name), &cloudwatch.LogGroupArgs{
		Tags: resourceTags(vpc.Args.BaseTags, pulumi.StringMap{
			"Name": pulumi.Sprintf("%s VPC Flow Logs", vpc.Args.Description),
		}),
	}, pulumi.Parent(flowLogsIAMRole))
	if err != nil {
		return err
	}

	// Role Policy
	rolepolicyJSON, err := json.Marshal(map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []interface{}{
			map[string]interface{}{
				"Action": []string{"" +
					"logs:CreateLogGroup",
					"logs:CreateLogStream",
					"logs:PutLogEvents",
					"logs:DescribeLogGroups",
					"logs:DescribeLogStreams",
				},
				"Effect":   "Allow",
				"Resource": "*",
			},
		},
	})
	if err != nil {
		return err
	}

	_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("%s-flow-log-policy", name), &iam.RolePolicyArgs{
		Name:   pulumi.String("vpc-flow-logs"),
		Role:   flowLogsIAMRole.ID(),
		Policy: pulumi.String(rolepolicyJSON),
	}, pulumi.Parent(flowLogsIAMRole))
	if err != nil {
		return err
	}

	_, err = ec2.NewFlowLog(ctx, fmt.Sprintf("%s-flow-logs", name), &ec2.FlowLogArgs{
		LogDestination: flowlogLogGroup.Arn,
		IamRoleArn:     flowLogsIAMRole.Arn,
		VpcId:          vpc.ID,
		TrafficType:    pulumi.String(trafficType),
	}, pulumi.Parent(flowLogsIAMRole))
	if err != nil {
		return err
	}

	return nil

}

func idOutputArrayToIDArrayOutput(as []pulumi.IDOutput) pulumi.IDArrayOutput {
	var outputs []interface{}
	for _, a := range as {
		outputs = append(outputs, a)
	}
	return pulumi.All(outputs...).ApplyIDArray(func(vs []interface{}) []pulumi.ID {
		var results []pulumi.ID
		for _, v := range vs {
			results = append(results, v.(pulumi.ID))
		}
		return results
	})
}
