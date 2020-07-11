package vpc

import (
	"fmt"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ec2"
)

type Vpc struct {
	pulumi.ResourceState

	ID pulumi.IDOutput `pulumi:"ID"`
	Cidr pulumi.StringOutput `pulumi:"Cidr"`
	Arn pulumi.StringOutput `pulumi:"Arn"`
}

type VpcArgs struct {
	BaseCidr pulumi.String
}

func NewVpc(ctx *pulumi.Context, name string, args VpcArgs, opts ...pulumi.ResourceOption) (*Vpc, error) {
	vpc := &Vpc{}

	// create the VPC
	awsVpc, err := ec2.NewVpc(ctx, fmt.Sprintf("%s-vpc", name), &ec2.VpcArgs{
		CidrBlock: args.BaseCidr,
		EnableDnsSupport: pulumi.Bool(true),
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

	// Register component resource
	err = ctx.RegisterComponentResource("jen20:aws-vpc", name, vpc, opts...)
	if err != nil {
		return nil, err
	}

	ctx.RegisterResourceOutputs(vpc, pulumi.Map{
		"ID": awsVpc.ID(),
		"Cidr": awsVpc.CidrBlock,
		"Arn": awsVpc.Arn,
	})

	return vpc, nil
}
