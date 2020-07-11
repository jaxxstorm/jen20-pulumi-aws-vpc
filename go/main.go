package vpc

import (
	"fmt"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
	"github.com/pulumi/pulumi-aws/sdk/v2/go/aws/ec2"
)

type Vpc struct {
	pulumi.ResourceState
}

type VpcArgs struct {
	BaseCidr pulumi.String
}

func NewVpc(ctx *pulumi.Context, name string, args VpcArgs, opts ...pulumi.ResourceOption) (*Vpc, error) {
	vpc := &Vpc{}

	_, err := ec2.NewVpc(ctx, fmt.Sprintf("%s-vpc", name), &ec2.VpcArgs{
		CidrBlock: args.BaseCidr,
		EnableDnsSupport: pulumi.Bool(true),
		EnableDnsHostnames: pulumi.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	
	// Register component resource
	err = ctx.RegisterComponentResource("jen20:aws-vpc", name, vpc, opts...)
	if err != nil {
		return nil, err
	}

	return vpc, nil
}
