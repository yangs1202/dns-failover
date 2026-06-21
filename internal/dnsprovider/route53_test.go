package dnsprovider

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

type fakeRoute53Client struct {
	input *route53.ChangeResourceRecordSetsInput
}

func (c *fakeRoute53Client) ChangeResourceRecordSets(_ context.Context, input *route53.ChangeResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
	c.input = input
	return &route53.ChangeResourceRecordSetsOutput{}, nil
}

func TestRoute53ProviderUpsertsCNAME(t *testing.T) {
	t.Parallel()

	client := &fakeRoute53Client{}
	provider := Route53Provider{
		hostedZoneID: "Z123",
		recordName:   "vip.example.invalid",
		recordType:   "CNAME",
		ttl:          30,
		client:       client,
	}

	if err := provider.UpdateCNAME(context.Background(), CNAMEChange{TargetName: "region.example.invalid"}); err != nil {
		t.Fatalf("UpdateCNAME returned error: %v", err)
	}

	if got := aws.ToString(client.input.HostedZoneId); got != "Z123" {
		t.Fatalf("expected hosted zone id Z123, got %q", got)
	}
	if len(client.input.ChangeBatch.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(client.input.ChangeBatch.Changes))
	}
	change := client.input.ChangeBatch.Changes[0]
	if change.Action != types.ChangeActionUpsert {
		t.Fatalf("expected UPSERT action, got %s", change.Action)
	}
	rrset := change.ResourceRecordSet
	if got := aws.ToString(rrset.Name); got != "vip.example.invalid." {
		t.Fatalf("expected record name with trailing dot, got %q", got)
	}
	if rrset.Type != types.RRTypeCname {
		t.Fatalf("expected CNAME type, got %s", rrset.Type)
	}
	if got := aws.ToInt64(rrset.TTL); got != 30 {
		t.Fatalf("expected ttl 30, got %d", got)
	}
	if got := aws.ToString(rrset.ResourceRecords[0].Value); got != "region.example.invalid." {
		t.Fatalf("expected target with trailing dot, got %q", got)
	}
}
