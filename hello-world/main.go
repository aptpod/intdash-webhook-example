package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

var (
	//go:embed intdash-webhook-secret
	intdashWebhookSecret string

	handler *Handler
)

func init() {
	var err error
	handler, err = provideLambdaHandler()
	if err != nil {
		log.Fatalf("[Error] Failed to provide lambda handler: %v", err)
	}
}

func main() {
	lambda.Start(handler.HandleAPIGatewayProxy)
}

func provideLambdaHandler() (*Handler, error) {
	snsTopicArn := os.Getenv("SNS_TOPIC_ARN")
	if snsTopicArn == "" {
		return nil, fmt.Errorf("SNS_TOPIC_ARN is not set")
	}

	awsCfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	return &Handler{
		IntdashAPI:    &IntdashAPIStub{},
		SHA256Key:     []byte(intdashWebhookSecret),
		SNSTopicArn:   snsTopicArn,
		SNSPublishAPI: sns.NewFromConfig(awsCfg),
	}, nil
}
