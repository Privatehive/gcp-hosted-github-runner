package test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Tereius/gcp-hosted-github-runner/pkg"
	"github.com/stretchr/testify/assert"
)

var PORT = 9999

var scaler pkg.Autoscaler

const PROJECT_ID = "github-spot-runner"
const REGION = "europe-west1"
const ZONE = "europe-west1-c"
const GIT_HUB_ORG = "Privatehive"

func init() {

	scaler = pkg.NewAutoscaler(pkg.AutoscalerConfig{
		RouteCreateVm:    "/create",
		RouteDeleteVm:    "/delete",
		RouteWebhook:     "/webhook",
		WebhookSecret:    "It's a Secret to Everybody",
		ProjectId:        PROJECT_ID,
		Zone:             ZONE,
		TaskQueue:        "projects/" + PROJECT_ID + "/locations/" + REGION + "/queues/autoscaler-callback-queue",
		InstanceTemplate: "projects/" + PROJECT_ID + "/global/instanceTemplates/ephemeral-github-runner",
		SecretVersion:    "projects/" + PROJECT_ID + "/secrets/github-pat-token/versions/latest",
		RunnerPrefix:     "runner",
		RunnerGroupName:  "Default",
		RunnerGroupId:    1,
		RunnerLabels:     []string{"self-hosted"},
		GitHubOrg:        GIT_HUB_ORG,
	})
	go scaler.Srv(PORT)
}

func TestWebhookSignature(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://127.0.0.1:%d/webhook", PORT), strings.NewReader("Hello, World!"))
	req.Header.Add("x-hub-signature-256", "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17")
	resp, err := http.DefaultClient.Do(req)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGenerateRunnerRegistrationToken(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	token, err := scaler.GenerateRunnerRegistrationToken(ctx)
	assert.Nil(t, err)
	assert.NotEmpty(t, token)
}

func TestGenerateRunnerJitConfig(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	jitConfig, err := scaler.GenerateRunnerJitConfig(ctx, "unit_test_test_runner_"+pkg.RandStringRunes(10))
	assert.Nil(t, err)
	assert.NotEmpty(t, jitConfig)
}
