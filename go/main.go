package vpc

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/route53"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

type Vpc struct {
	pulumi.ResourceState

	ID   pulumi.IDOutput     `pulumi:"ID"`
	Cidr pulumi.StringOutput `pulumi:"Cidr"`
	Arn  pulumi.StringOutput `pulumi:"Arn"`
}

type Args struct {
	BaseCidr string
	ZoneName pulumi.String
	AvailabilityZoneNames pulumi.StringArray
}

func NewVpc(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Vpc, error) {
	vpc := &Vpc{}

	// create the VPC
	awsVpc, err := ec2.NewVpc(ctx, fmt.Sprintf("%s-vpc", name), &ec2.VpcArgs{
		CidrBlock:          pulumi.String(args.BaseCidr),
		EnableDnsSupport:   pulumi.Bool(true),
		EnableDnsHostnames: pulumi.Bool(true),
	}, pulumi.Parent(vpc))
	if err != nil {
		return nil, err
	}

	// export some VPC outputs
	vpc.ID = awsVpc.ID()
	vpc.Cidr = awsVpc.CidrBlock
	vpc.Arn = awsVpc.Arn

	// add an internet gateway
	_, err = ec2.NewInternetGateway(ctx, fmt.Sprintf("%s-igw", name), &ec2.InternetGatewayArgs{
		VpcId: awsVpc.ID(),
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

	privateSubnets, publicSubnets, err := SubnetDistributor(args.BaseCidr, len(args.AvailabilityZoneNames))
	if err != nil {
		return nil, err
	}

	for index, subnet := range privateSubnets {
		_, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-private-%d", name, index+1), &ec2.SubnetArgs{
			VpcId: awsVpc.ID(),
			CidrBlock: pulumi.String(subnet),
			AvailabilityZone: args.AvailabilityZoneNames[index],
		}, pulumi.Parent(awsVpc))
		if err != nil {
			return nil, err
		}
	}

	for index, subnet := range publicSubnets {
		_, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-public-%d", name, index+1), &ec2.SubnetArgs{
			VpcId: awsVpc.ID(),
			CidrBlock: pulumi.String(subnet),
			MapPublicIpOnLaunch: pulumi.Bool(true),
			AvailabilityZone: args.AvailabilityZoneNames[index],
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
		"ID":   awsVpc.ID(),
		"Cidr": awsVpc.CidrBlock,
		"Arn":  awsVpc.Arn,
	})

	return vpc, nil
}
