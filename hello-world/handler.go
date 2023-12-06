package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

const (
	// IntdashSignatureHeader is the name of the header that contains the signature.
	// The signature is a SHA256 hash of the request body and base64 encoded.
	IntdashSignatureHeader = "x-intdash-signature-256"
)

type (
	IntdashAPI interface {
		FetchFloat64DataPoints(ctx context.Context, measurementUUID string) ([]float64, error)
	}

	SNSPublishAPI interface {
		Publish(ctx context.Context, input *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
	}

	Handler struct {
		IntdashAPI    IntdashAPI
		SHA256Key     []byte
		SNSPublishAPI SNSPublishAPI
		SNSTopicArn   string
	}
)

// HandleAPIGatewayProxy handles the API Gateway Proxy request of intdash webhook.
func (h *Handler) HandleAPIGatewayProxy(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log.Printf("[Info] Got request: %v", request)

	if err := h.validateSignature(ctx, request); err != nil {
		log.Printf("[Error] Got invalid signature: %v", err)
		return events.APIGatewayProxyResponse{
			Body:       "Invalid signature",
			StatusCode: http.StatusBadRequest,
		}, nil
	}

	body, err := h.extractWebhookBody(ctx, request)
	if err != nil {
		log.Printf("[Error] Got invalid request body: %v", err)
		return events.APIGatewayProxyResponse{
			Body:       "Invalid request body",
			StatusCode: http.StatusBadRequest,
		}, nil
	}
	if !(body.ResourceType == "measurement" && body.Action == "completed") {
		log.Printf("[Info] Got unsupported resource type or action: %v", err)
		return events.APIGatewayProxyResponse{
			Body:       "Unsupported resource type or action",
			StatusCode: http.StatusUnprocessableEntity,
		}, nil
	}

	dataPoints, err := h.IntdashAPI.FetchFloat64DataPoints(ctx, body.MeasurementUUID)
	if err != nil {
		log.Printf("[Error] Failed to fetch data points: %v", err)
		return events.APIGatewayProxyResponse{
			Body:       "Failed to fetch data points",
			StatusCode: http.StatusInternalServerError,
		}, nil
	}

	notificationBody := h.makeNotificationBody(dataPoints)
	if err := h.PublishSNS(ctx, notificationBody); err != nil {
		log.Printf("[Error] Failed to publish SNS: %v", err)
		return events.APIGatewayProxyResponse{
			Body:       "Failed to publish SNS",
			StatusCode: http.StatusInternalServerError,
		}, nil
	}

	return events.APIGatewayProxyResponse{
		Body:       "",
		StatusCode: http.StatusNoContent,
	}, nil
}

// validateSignature validates the signature of the given request.
func (h *Handler) validateSignature(ctx context.Context, request events.APIGatewayProxyRequest) error {
	signature := request.Headers[IntdashSignatureHeader]
	if signature == "" {
		return fmt.Errorf("signature header %q is empty", IntdashSignatureHeader)
	}
	wantSum, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	hasher := hmac.New(sha256.New, h.SHA256Key)
	if _, err := hasher.Write([]byte(request.Body)); err != nil {
		return fmt.Errorf("write body to hasher: %w", err)
	}
	sum := hasher.Sum(nil)

	if !hmac.Equal(wantSum, sum) {
		return fmt.Errorf("signature mismatch, want %x, got %x", wantSum, sum)
	}

	return nil
}

// WebhookBody is the body of the webhook request.
type WebhookBody struct {
	ResourceType    string `json:"resource_type"`
	Action          string `json:"action"`
	MeasurementUUID string `json:"measurement_uuid"`
}

// extractWebhookBody extracts the webhook body from the given request.
func (h *Handler) extractWebhookBody(ctx context.Context, request events.APIGatewayProxyRequest) (*WebhookBody, error) {
	var body WebhookBody
	if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
		return nil, fmt.Errorf("unmarshal request body: %w", err)
	}
	return &body, nil
}

// makeNotificationBody makes a notification body from the given data points.
// The body contains the average and the unbiased variance.
func (h *Handler) makeNotificationBody(dataPoints []float64) string {
	var sum float64
	for _, v := range dataPoints {
		sum += v
	}
	avg := sum / float64(len(dataPoints))

	var variance float64
	if len(dataPoints) > 1 {
		var dss float64 // deviation sum of squares
		for _, v := range dataPoints {
			dss += (v - avg) * (v - avg)
		}
		variance = dss / float64(len(dataPoints)-1)
	}

	return fmt.Sprintf("Average: %f\n"+"Unbiased Variance: %f\n", avg, variance)
}

// PublishSNS publishes the given body to SNS.
func (h *Handler) PublishSNS(ctx context.Context, body string) error {
	input := &sns.PublishInput{
		TopicArn: aws.String(h.SNSTopicArn),
		Message:  &body,
	}
	out, err := h.SNSPublishAPI.Publish(ctx, input)
	if err != nil {
		return fmt.Errorf("publish SNS: %w", err)
	}
	log.Printf("[Info] Published SNS: %s", *out.MessageId)
	return nil
}
