package dnsprovider

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

type route53API interface {
	ChangeResourceRecordSets(context.Context, *route53.ChangeResourceRecordSetsInput, ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
}

type Route53Provider struct {
	hostedZoneID string
	recordName   string
	recordType   string
	ttl          int
	client       route53API
}

func NewRoute53Provider(cfg Config) (Provider, error) {
	if strings.TrimSpace(cfg.ZoneID) == "" {
		return nil, fmt.Errorf("route53 hosted zone id is required")
	}
	if strings.TrimSpace(cfg.RecordName) == "" {
		return nil, fmt.Errorf("route53 record name is required")
	}

	recordType := strings.TrimSpace(cfg.RecordType)
	if recordType == "" {
		recordType = "CNAME"
	}
	if recordType != "CNAME" {
		return nil, fmt.Errorf("route53 provider only supports CNAME records, got %q", recordType)
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 60
	}

	loadOptions := []func(*config.LoadOptions) error{
		config.WithRegion("us-east-1"),
	}
	if strings.TrimSpace(cfg.AccessKeyID) != "" || strings.TrimSpace(cfg.SecretAccessKey) != "" {
		if strings.TrimSpace(cfg.AccessKeyID) == "" || strings.TrimSpace(cfg.SecretAccessKey) == "" {
			return nil, fmt.Errorf("route53 static credentials require both access key id and secret access key")
		}
		loadOptions = append(loadOptions, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)))
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config for route53: %w", err)
	}

	return Route53Provider{
		hostedZoneID: strings.TrimSpace(cfg.ZoneID),
		recordName:   strings.TrimSuffix(strings.TrimSpace(cfg.RecordName), "."),
		recordType:   recordType,
		ttl:          ttl,
		client:       route53.NewFromConfig(awsCfg),
	}, nil
}

func (p Route53Provider) UpdateCNAME(ctx context.Context, change CNAMEChange) error {
	hostedZoneID := p.hostedZoneID
	if change.ZoneID != "" {
		hostedZoneID = strings.TrimSpace(change.ZoneID)
	}
	recordName := p.recordName
	if change.RecordName != "" {
		recordName = strings.TrimSuffix(strings.TrimSpace(change.RecordName), ".")
	}

	_, err := p.client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String(ensureTrailingDot(recordName)),
						Type: types.RRType(p.recordType),
						TTL:  aws.Int64(int64(p.ttl)),
						ResourceRecords: []types.ResourceRecord{
							{
								Value: aws.String(ensureTrailingDot(change.TargetName)),
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("route53 change resource record sets: %w", err)
	}

	return nil
}
