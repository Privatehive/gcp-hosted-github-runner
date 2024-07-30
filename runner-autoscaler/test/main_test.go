package test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Tereius/g-spot-runner-github-actions/pkg"
	"github.com/stretchr/testify/assert"
)

var PORT = 9999

func init() {

	scaler := pkg.NewAutoscaler(pkg.AutoscalerConfig{
		RouteCreateVm:    "/create",
		RouteDeleteVm:    "/delete",
		RouteWebhook:     "/webhook",
		WebhookSecret:    "It's a Secret to Everybody",
		ProjectId:        "1",
		Zone:             "z",
		TaskQueue:        "q",
		InstanceTemplate: "/",
		RunnerPrefix:     "runner",
	})
	go scaler.Srv(PORT)
}

func TestWebhookSignature(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://localhost:%d/webhook", PORT), strings.NewReader("Hello, World!"))
	req.Header.Add("x-hub-signature-256", "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17")
	resp, err := http.DefaultClient.Do(req)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
}
