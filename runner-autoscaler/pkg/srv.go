package pkg

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	ginlogrus "github.com/toorop/gin-logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const GITHUB_API_VERSION string = "2022-11-28"
const SHA_PREFIX string = "sha256="
const SHA_HEADER string = "x-hub-signature-256"
const EVENT_HEADER string = "x-github-event"

const WEBHOOK_PING_EVENT string = "ping"
const WEBHOOK_JOB_EVENT string = "workflow_job"

const RUNNER_REGISTRATION_TOKEN_ATTR string = "registration_token"
const RUNNER_JIT_CONFIG_ATTR string = "jit_config"

const RUNNER_SCRIPT_REGISTER_RUNNER_ATTR string = "startup_script_register_runner"         // has to match the global custom metadata in compute.tf
const RUNNER_SCRIPT_REGISTER_JIT_RUNNER_ATTR string = "startup_script_register_jit_runner" // has to match the global custom metadata in compute.tf

const RUNNER_REGISTER_TOKEN_ORG_ENDPOINT string = "https://api.github.com/orgs/%s/actions/runners/registration-token"

const RUNNER_ENTERPRISE_JIT_CONFIG_ENDPOINT string = "https://api.github.com/enterprises/%s/actions/runners/generate-jitconfig"
const RUNNER_ORG_JIT_CONFIG_ENDPOINT string = "https://api.github.com/orgs/%s/actions/runners/generate-jitconfig"
const RUNNER_REPO_JIT_CONFIG_ENDPOINT string = "https://api.github.com/repos/%s/actions/runners/generate-jitconfig" // format USER/REPO

type SourceType string

const (
	TypeEnterprise   SourceType = "enterprise"
	TypeOrganization SourceType = "organization"
	TypeRepository   SourceType = "repository"
)

type Source struct {
	Name       string     `json:"name"`
	SourceType SourceType `json:"type"`
	Secret     string     `json:"secret"`
}

type Job struct {
	Id              int64    `json:"id"`
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	Labels          []string `json:"labels"`
	RunnerName      string   `json:"runner_name"`
	RunnerGroupName string   `json:"runner_group_name"`
	RunnerGroupId   int64    `json:"runner_group_id"`
}

type Payload struct {
	Action Action `json:"action"`
	Job    Job    `json:"workflow_job"`
}

type VmSettings struct {
	Name        string  `json:"name"`
	MachineType *string `json:"machineType,omitempty"`
}

func (j Job) hasLabel(label string) bool {

	for _, l := range j.Labels {
		if l == label {
			return true
		}
	}
	return false
}

type MagicLabel string

const (
	MagicLabelMachine MagicLabel = "machine"
)

var magicLabels = []string{string(MagicLabelMachine)}
var matchMagicLabels = regexp.MustCompile(`@(` + strings.Join(magicLabels, "|") + `):`)

func IsMagicLabel(label string) bool {

	if matches := matchMagicLabels.FindStringSubmatch(label); len(matches) >= 2 {
		return true
	}
	return false
}

func (j Job) GetMagicLabelValue(key MagicLabel) *string {

	matchMagicLabel := regexp.MustCompile("@(" + string(key) + "):(.+)")
	for _, l := range j.Labels {
		matches := matchMagicLabel.FindStringSubmatch(l)
		if len(matches) >= 3 {
			ret := matches[2]
			return &ret
		}
	}
	return nil
}

// returns true if all labels were found (excluding magic labels). false otherwise. Returns also all labels that were missing
func (j Job) HasAllLabels(labels []string) (bool, []string) {

	missingLabels := []string{}
	for _, label := range labels {
		if !IsMagicLabel(label) {
			if !j.hasLabel(label) {
				missingLabels = append(missingLabels, label)
			}
		}
	}
	return len(missingLabels) <= 0, missingLabels
}

type Action string

const (
	QUEUED      Action = "queued"
	COMPLETED   Action = "completed"
	IN_PROGRESS Action = "in_progress"
	WAITING     Action = "waiting"
)

type State string

const (
	// running
	PROVISIONING State = "PROVISIONING" // resources are allocated for the VM. The VM is not running yet.
	STAGING      State = "STAGING"      // resources are acquired, and the VM is preparing for first boot.
	RUNNING      State = "RUNNING"      // the VM is booting up or running.
	// stopped
	STOPPING   State = "STOPPING"   // the VM is being stopped. You requested a stop, or a failure occurred. This is a temporary status after which the VM enters the TERMINATED status.
	SUSPENDING State = "SUSPENDING" // the VM is in the process of being suspended. You suspended the VM.
	SUSPENDED  State = "SUSPENDED"  // the VM is in a suspended state. You can resume the VM or delete it.
	TERMINATED State = "TERMINATED" // the VM is stopped. You stopped the VM, or the VM encountered a failure. You can restart or delete the VM.
	// should result in running state
	REPAIRING State = "REPAIRING" // the VM is being repaired. Repairing occurs when the VM encounters an internal error or the underlying machine is unavailable due to maintenance. During this time, the VM is unusable. You are not billed when a VM is in repair. VMs are not covered by the Service level agreement (SLA) while they are in repair. If repair succeeds, the VM returns to one of the above states.
	Unknown   State = "unknown"
)

/*
func (s State) isStopped() bool {

	return s == STOPPING || s == SUSPENDING || s == SUSPENDED || s == TERMINATED
}

func (s State) isRunning() bool {

	return s == PROVISIONING || s == STAGING || s == RUNNING || s == REPAIRING
}*/

type InstanceClient struct {
	*compute.InstancesClient
}

func createCallbackUrl(ctx *gin.Context, path string, srcQueryName string, srcQueryValue string) string {

	return "https://" + ctx.Request.Host + path + "?" + srcQueryName + "=" + url.QueryEscape(srcQueryValue)
}

func newComputeClient(ctx context.Context) *InstanceClient {

	if client, err := compute.NewInstancesRESTClient(ctx); err != nil {
		panic(err)
	} else {
		return &InstanceClient{client}
	}
}

func newTaskClient(ctx context.Context) *cloudtasks.Client {

	if client, err := cloudtasks.NewClient(ctx); err != nil {
		panic(err)
	} else {
		return client
	}
}

func newSecretAccessClient(ctx context.Context) *secretmanager.Client {

	if client, err := secretmanager.NewClient(ctx); err != nil {
		panic(err)
	} else {
		return client
	}
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")

func RandStringRunes(n int) string {

	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func calcSigHex(secret []byte, data []byte) string {

	sig := hmac.New(sha256.New, secret)
	sig.Write(data)
	return hex.EncodeToString(sig.Sum(nil))
}

// returns http body, "src" query, error
func (s *Autoscaler) verifySignature(ctx *gin.Context) ([]byte, Source, error) {

	if signature := ctx.GetHeader(SHA_HEADER); len(signature) == 71 {
		if body, err := io.ReadAll(ctx.Request.Body); err != nil {
			log.Errorf("Error receiving http body: %s", err.Error())
			return nil, Source{}, ctx.AbortWithError(http.StatusBadRequest, err)
		} else {
			if src, ok := ctx.GetQuery(s.conf.SourceQueryParam); ok {
				if source, ok := s.conf.RegisteredSources[src]; ok {
					if calcSignature := calcSigHex([]byte(source.Secret), body); calcSignature == signature[7:] {
						return body, source, nil
					} else {
						log.Warnf("%s signature did not match", ctx.RemoteIP())
						return nil, Source{}, ctx.AbortWithError(http.StatusUnauthorized, fmt.Errorf("unauthorized"))
					}
				} else {
					log.Infof("Source with name %s not registered - ignoring", src)
					ctx.Status(http.StatusOK) // not considered an error
					return nil, Source{}, fmt.Errorf("unknown webhook source")
				}
			} else {
				log.Errorf("Missing %s query parameter", s.conf.SourceQueryParam)
				return nil, Source{}, ctx.AbortWithError(http.StatusBadRequest, fmt.Errorf("missing %s query parameter", s.conf.SourceQueryParam))
			}
		}
	} else {
		log.Warnf("%s did not provide a signature", ctx.RemoteIP())
		return nil, Source{}, ctx.AbortWithError(http.StatusUnauthorized, fmt.Errorf("unauthorized"))
	}
}

func (s *Autoscaler) GetInstanceState(ctx context.Context, instanceName string) (State, error) {

	client := newComputeClient(ctx)
	defer client.Close()
	if res, err := client.Get(ctx, &computepb.GetInstanceRequest{
		Project:  s.conf.ProjectId,
		Zone:     s.conf.Zone,
		Instance: instanceName,
	}); err != nil {
		log.Errorf("Could not get status for instance: %s - %s", instanceName, err.Error())
		return Unknown, err
	} else if res.Status == nil {
		log.Errorf("Could not read status for instance: %s", instanceName)
		return Unknown, fmt.Errorf("instance status is unknown")
	} else {
		return (State)(*res.Status), nil
	}
}

// blocking until instance started or failed to start
func (s *Autoscaler) StartInstance(ctx context.Context, instanceName string) error {

	log.Infof("About to start instance: %s", instanceName)
	client := newComputeClient(ctx)
	defer client.Close()
	if res, err := client.Start(ctx, &computepb.StartInstanceRequest{
		Project:  s.conf.ProjectId,
		Zone:     s.conf.Zone,
		Instance: instanceName,
	}); err != nil {
		log.Errorf("Could not start instance: %s - %s", instanceName, err.Error())
		return err
	} else {
		if err := res.Wait(ctx); err != nil {
			log.Errorf("Failed to wait for instance to start: %s", err.Error())
			return err
		} else {
			log.Infof("Started instance: %s", instanceName)
		}
	}
	return nil
}

// blocking until instance stopped or failed to stop
func (s *Autoscaler) StopInstance(ctx context.Context, instanceName string) error {

	log.Debugf("About to stop instance: %s", instanceName)
	client := newComputeClient(ctx)
	defer client.Close()
	if res, err := client.Stop(ctx, &computepb.StopInstanceRequest{
		Project:  s.conf.ProjectId,
		Zone:     s.conf.Zone,
		Instance: instanceName,
	}); err != nil {
		log.Errorf("Could not stop instance: %s - %s", instanceName, err.Error())
		return err
	} else {
		if err := res.Wait(ctx); err != nil {
			log.Errorf("Failed to wait for instance to stop: %s", err.Error())
			return err
		} else {
			log.Infof("Stopped instance: %s", instanceName)
		}
	}
	return nil
}

// blocking until instance started or failed to start
func (s *Autoscaler) DeleteInstance(ctx context.Context, instanceName string) error {

	log.Debugf("About to delete instance: %s", instanceName)
	client := newComputeClient(ctx)
	defer client.Close()
	if res, err := client.Delete(ctx, &computepb.DeleteInstanceRequest{
		Project:  s.conf.ProjectId,
		Zone:     s.conf.Zone,
		Instance: instanceName,
	}); err != nil {
		log.Errorf("Could not delete instance: %s - %s", instanceName, err.Error())
		return err
	} else {
		if err := res.Wait(ctx); err != nil {
			log.Errorf("Failed to wait for instance to be deleted: %s", err.Error())
			return err
		} else {
			log.Infof("Deleted instance: %s", instanceName)
		}
	}
	return nil
}

// blocking until instance started or failed to start
func (s *Autoscaler) CreateInstanceFromTemplate(ctx context.Context, instanceName string, machineType *string, metadata ...*computepb.Items) error {

	log.Debugf("About to create instance %s from template", instanceName)
	computeClient := newComputeClient(ctx)
	defer computeClient.Close()

	var machine *string = nil
	if machineType != nil {
		machine = proto.String(fmt.Sprintf("zones/%s/machineTypes/%s", s.conf.Zone, *machineType))
	}

	if res, err := computeClient.Insert(ctx, &computepb.InsertInstanceRequest{
		Project: s.conf.ProjectId,
		Zone:    s.conf.Zone,
		InstanceResource: &computepb.Instance{
			Name:        proto.String(instanceName),
			MachineType: machine,
			Metadata: &computepb.Metadata{
				Items: metadata,
			},
		},
		SourceInstanceTemplate: &s.conf.InstanceTemplate,
	}); err != nil {
		log.Errorf("Could not create instance %s from template: %s - %s", instanceName, s.conf.InstanceTemplate, err.Error())
		return err
	} else {
		if err := res.Wait(ctx); err != nil {
			log.Errorf("Failed to wait for instance to be created from template: %s", err.Error())
			return err
		} else {
			log.Infof("Created instance from template: %s", instanceName)
		}
	}
	return nil
}

func (s *Autoscaler) readPat(ctx context.Context) (string, error) {

	log.Debugf("About to read PAT from secret version: %s", s.conf.SecretVersion)
	secretAccessClient := newSecretAccessClient(ctx)
	defer secretAccessClient.Close()
	if secretResult, err := secretAccessClient.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: s.conf.SecretVersion,
	}); err != nil {
		log.Errorf("Could not access GitHub PAT secret version %s: %s", s.conf.SecretVersion, err.Error())
		return "", fmt.Errorf("missing GitHub PAT")
	} else {
		if pat := string(secretResult.Payload.Data); len(pat) == 0 {
			log.Errorf("The GitHub PAT secret is empty")
			return "", fmt.Errorf("empty GitHub PAT")
		} else {
			return pat, nil
		}
	}
}

/*
func (s *Autoscaler) GenerateRunnerRegistrationToken(ctx context.Context) (string, error) {

	log.Debugf("About to request GitHub runner registration token using PAT from secret version: %s", s.conf.SecretVersion)
	secretAccessClient := newSecretAccessClient(ctx)
	defer secretAccessClient.Close()
	if pat, err := s.readPat(ctx); err != nil {
		return "", err
	} else {
		if req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf(RUNNER_REGISTER_TOKEN_ORG_ENDPOINT, s.conf.GitHubOrg), nil); err != nil {
			log.Errorf("Could not create GitHub runner registration token request")
			return "", fmt.Errorf("failed registration token request")
		} else {
			req.Header.Add("Accept", "application/vnd.github+json")
			req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", pat))
			req.Header.Add("X-GitHub-Api-Version", GITHUB_API_VERSION)
			req.Header.Add("User-Agent", "github-runner-autoscaler")
			if resp, err := http.DefaultClient.Do(req); err != nil {
				log.Errorf("GitHub runner registration token request failed: %s", err.Error())
				return "", fmt.Errorf("failed registration token response")
			} else if resp.StatusCode != 201 {
				log.Errorf("GitHub runner registration token request unsuccessful: %s", resp.Status)
				defer resp.Body.Close()
				return "", fmt.Errorf("failed registration token response")
			} else {
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				payload := map[string]string{}
				if err := json.Unmarshal(body, &payload); err != nil {
					log.Errorf("GitHub runner registration token response missing: %s", err.Error())
					return "", fmt.Errorf("failed registration token response")
				} else if token, ok := payload["token"]; ok && len(token) > 0 {
					return token, nil
				} else {
					log.Errorf("GitHub runner registration token is empty")
					return "", fmt.Errorf("failed registration token response")
				}
			}
		}
	}
}*/

// A jit-config needs: RunnerName, RunnerGroupId, Labels, WorkFolder
func (s *Autoscaler) GenerateRunnerJitConfig(ctx context.Context, url string, runnerName string, runnerGroupId int64, labels []string) (string, error) {

	log.Debugf("About to request GitHub runner %s jit config from %s (runner group %d) using PAT from secret version: %s", runnerName, url, runnerGroupId, s.conf.SecretVersion)
	secretAccessClient := newSecretAccessClient(ctx)
	defer secretAccessClient.Close()
	if pat, err := s.readPat(ctx); err != nil {
		return "", err
	} else {
		reqPayload := map[string]any{}
		reqPayload["name"] = runnerName
		reqPayload["runner_group_id"] = runnerGroupId
		reqPayload["labels"] = labels
		reqPayload["work_folder"] = "_work"
		data, _ := json.Marshal(reqPayload)
		if req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data)); err != nil {
			log.Errorf("Could not create GitHub runner jit-config request")
			return "", fmt.Errorf("failed jit-config request")
		} else {
			req.Header.Add("Accept", "application/vnd.github+json")
			req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", pat))
			req.Header.Add("X-GitHub-Api-Version", GITHUB_API_VERSION)
			req.Header.Add("User-Agent", "github-runner-autoscaler")
			if resp, err := http.DefaultClient.Do(req); err != nil {
				log.Errorf("GitHub runner jit-config request failed: %s", err.Error())
				return "", fmt.Errorf("failed jit-config response")
			} else if resp.StatusCode != 201 {
				log.Errorf("GitHub runner jit-config request unsuccessful: %s", resp.Status)
				defer resp.Body.Close()
				return "", fmt.Errorf("failed jit-config response")
			} else {
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				payload := map[string]any{}
				if err := json.Unmarshal(body, &payload); err != nil {
					log.Errorf("GitHub runner jit-config response missing: %s", err.Error())
					return "", fmt.Errorf("failed jit-config response")
				} else if jitConfig, ok := payload["encoded_jit_config"].(string); ok && len(jitConfig) > 0 {
					return jitConfig, nil
				} else {
					log.Errorf("GitHub runner jit-config is empty")
					return "", fmt.Errorf("failed jit-config response")
				}
			}
		}
	}
}

func (s *Autoscaler) createCallbackTaskWithToken(ctx context.Context, url string, secret string, job Job) error {

	data, _ := json.Marshal(job)
	now := timestamppb.Now()
	now.Seconds += 1 // delay the callback a little bit
	req := &taskspb.CreateTaskRequest{
		Parent: s.conf.TaskQueue,
		Task: &taskspb.Task{
			DispatchDeadline: &durationpb.Duration{
				Seconds: 120, // the timeout of the cloud task callback
				Nanos:   0,
			},
			ScheduleTime: now,
			MessageType: &taskspb.Task_HttpRequest{
				HttpRequest: &taskspb.HttpRequest{
					HttpMethod: taskspb.HttpMethod_POST,
					Url:        url,
					Headers: map[string]string{
						SHA_HEADER: SHA_PREFIX + calcSigHex([]byte(secret), []byte(data)),
					},
				},
			},
		},
	}

	req.Task.GetHttpRequest().Body = []byte(data)

	client := newTaskClient(ctx)
	defer client.Close()
	_, err := client.CreateTask(ctx, req)
	if err != nil {
		return fmt.Errorf("cloudtasks.CreateTask failed: %v", err)
	} else {
		log.Infof("Created cloud task callback with url \"%s\" and payload \"%s\"", url, data)
	}

	return nil
}

const runner_script_wrapper = `
#!/bin/bash
val=$(curl "http://metadata.google.internal/computeMetadata/v1/instance/attributes/%s" -H "Metadata-Flavor: Google")
curl "http://metadata.google.internal/computeMetadata/v1/project/attributes/%s" -H "Metadata-Flavor: Google" > runner_startup.sh
chmod +x ./runner_startup.sh
./runner_startup.sh $val
rm runner_startup.sh
`

/*
func (s *Autoscaler) createVmWithRegistrationToken(ctx *gin.Context, instanceName string) {

	if token, err := s.GenerateRunnerRegistrationToken(ctx); err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err)
	} else {
		registration_token_attr := fmt.Sprintf("%s_%s", RUNNER_REGISTRATION_TOKEN_ATTR, RandStringRunes(16))
		if err := s.CreateInstanceFromTemplate(ctx, instanceName, &computepb.Items{
			Key:   proto.String(registration_token_attr),
			Value: proto.String(token),
		}, &computepb.Items{
			Key:   proto.String("startup-script"),
			Value: proto.String(fmt.Sprintf(runner_script_wrapper, registration_token_attr, RUNNER_SCRIPT_REGISTER_RUNNER_ATTR)),
		}); err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
		} else {
			ctx.Status(http.StatusOK)
		}
	}
}*/

func (s *Autoscaler) createVmWithJitConfig(ctx *gin.Context, url string, runnerGroupId int64, settings VmSettings, labels []string) {

	if jitConfig, err := s.GenerateRunnerJitConfig(ctx, url, settings.Name, runnerGroupId, labels); err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err)
	} else {
		jit_config_attr := fmt.Sprintf("%s_%s", RUNNER_JIT_CONFIG_ATTR, RandStringRunes(16))
		if err := s.CreateInstanceFromTemplate(ctx, settings.Name, settings.MachineType, &computepb.Items{
			Key:   proto.String(jit_config_attr),
			Value: proto.String(jitConfig),
		}, &computepb.Items{
			Key:   proto.String("startup-script"),
			Value: proto.String(fmt.Sprintf(runner_script_wrapper, jit_config_attr, RUNNER_SCRIPT_REGISTER_JIT_RUNNER_ATTR)),
		}); err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
		} else {
			ctx.Status(http.StatusOK)
		}
	}
}

func (s *Autoscaler) handleCreateVm(ctx *gin.Context) {

	log.Info("Received create-vm cloud task callback")
	if data, src, err := s.verifySignature(ctx); err == nil {
		job := Job{}
		json.Unmarshal(data, &job)
		// use jit config
		switch src.SourceType {
		case TypeEnterprise:
			log.Infof("Using jit config for runner registration for enterprise: %s", src.Name)
			s.createVmWithJitConfig(ctx, fmt.Sprintf(RUNNER_ENTERPRISE_JIT_CONFIG_ENDPOINT, src.Name), s.conf.RunnerGroupId, VmSettings{
				Name:        fmt.Sprintf("%s-%s", s.conf.RunnerPrefix, RandStringRunes(10)),
				MachineType: job.GetMagicLabelValue(MagicLabelMachine),
			}, job.Labels)
		case TypeOrganization:
			log.Infof("Using jit config for runner registration for organization: %s", src.Name)
			s.createVmWithJitConfig(ctx, fmt.Sprintf(RUNNER_ORG_JIT_CONFIG_ENDPOINT, src.Name), s.conf.RunnerGroupId, VmSettings{
				Name:        fmt.Sprintf("%s-%s", s.conf.RunnerPrefix, RandStringRunes(10)),
				MachineType: job.GetMagicLabelValue(MagicLabelMachine),
			}, job.Labels)
		case TypeRepository:
			log.Infof("Using jit config for runner registration for repository: %s", src.Name)
			// For repositories there is an implicit runner group with id 1
			s.createVmWithJitConfig(ctx, fmt.Sprintf(RUNNER_REPO_JIT_CONFIG_ENDPOINT, src.Name), 1, VmSettings{
				Name:        fmt.Sprintf("%s-%s", s.conf.RunnerPrefix, RandStringRunes(10)),
				MachineType: job.GetMagicLabelValue(MagicLabelMachine),
			}, job.Labels)
		default:
			log.Errorf("Missing source type for %s", src.Name)
			ctx.Status(http.StatusBadRequest)
		}
	}
}

func (s *Autoscaler) handleDeleteVm(ctx *gin.Context) {

	log.Info("Received delete-vm cloud task callback")
	if data, _, err := s.verifySignature(ctx); err == nil {
		job := Job{}
		json.Unmarshal(data, &job)
		if err := s.DeleteInstance(ctx, job.RunnerName); err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
		} else {
			ctx.Status(http.StatusOK)
		}
	}
}

func (s *Autoscaler) handleWebhook(ctx *gin.Context) {

	log.Info("Received webhook")
	if data, src, err := s.verifySignature(ctx); err == nil {
		event := ctx.GetHeader(EVENT_HEADER)
		log.Info(string(data))
		if event == WEBHOOK_PING_EVENT {
			log.Info("Webhook ping acknowledged")
			ctx.Status(http.StatusOK)
		} else if event == WEBHOOK_JOB_EVENT {
			payload := Payload{}
			if err := json.Unmarshal(data, &payload); err != nil {
				log.Errorf("Can not unmarshal payload - is the webhook content type set to \"application/json\"? %s", err.Error())
				ctx.AbortWithError(http.StatusBadRequest, err)
			} else {
				if payload.Action == QUEUED {
					if ok, missingLabels := payload.Job.HasAllLabels(s.conf.RunnerLabels); ok {
						createUrl := createCallbackUrl(ctx, s.conf.RouteCreateVm, s.conf.SourceQueryParam, src.Name)
						if err := s.createCallbackTaskWithToken(ctx, createUrl, src.Secret, payload.Job); err != nil {
							log.Errorf("Can not enqueue create-vm cloud task callback: %s", err.Error())
							ctx.AbortWithError(http.StatusInternalServerError, err)
							return
						}
					} else {
						log.Warnf("Webhook requested to start a runner that is missing the label(s) \"%s\" - ignoring", strings.Join(missingLabels, ", "))
					}
				} else if payload.Action == COMPLETED {
					runnerGroupId := s.conf.RunnerGroupId
					if src.SourceType == TypeRepository {
						runnerGroupId = 1
					}
					if payload.Job.RunnerGroupId == runnerGroupId {
						if ok, missingLabels := payload.Job.HasAllLabels(s.conf.RunnerLabels); ok {
							deleteUrl := createCallbackUrl(ctx, s.conf.RouteDeleteVm, s.conf.SourceQueryParam, src.Name)
							if err := s.createCallbackTaskWithToken(ctx, deleteUrl, src.Secret, payload.Job); err != nil {
								log.Errorf("Can not enqueue delete-vm cloud task callback: %s", err.Error())
								ctx.AbortWithError(http.StatusInternalServerError, err)
								return
							}
						} else {
							log.Warnf("Webhook signaled to delete a runner that is missing the label(s) \"%s\" - ignoring", strings.Join(missingLabels, ", "))
						}
					} else {
						log.Warnf("Webhook signaled to delete a runner that does not belong to the expected runner group (expected \"%d\" got \"%d\") - ignoring", runnerGroupId, payload.Job.RunnerGroupId)
					}
				}
				ctx.Status(http.StatusOK)
			}
		} else {
			log.Infof("Unknown GitHub webhook event \"%s\" received - ignoring", event)
			ctx.Status(http.StatusOK)
		}
	}
}

type Pair struct {
	Name   string
	Secret string
}

func (p Pair) IsIValid() bool {
	return len(p.Name) > 0 && len(p.Secret) > 0
}

type AutoscalerConfig struct {
	RouteWebhook      string
	RouteCreateVm     string
	RouteDeleteVm     string
	ProjectId         string
	Zone              string
	TaskQueue         string
	InstanceTemplate  string
	SecretVersion     string
	RunnerPrefix      string
	RunnerGroupId     int64
	RunnerLabels      []string
	RegisteredSources map[string]Source
	SourceQueryParam  string
}

type Autoscaler struct {
	engine *gin.Engine
	conf   AutoscalerConfig
}

func NewAutoscaler(config AutoscalerConfig) *Autoscaler {

	engine := gin.New()

	scaler := Autoscaler{
		engine: engine,
		conf:   config,
	}
	engine.Use(ginlogrus.Logger(log.WithFields(log.Fields{})))
	engine.POST(config.RouteCreateVm, scaler.handleCreateVm)
	engine.POST(config.RouteDeleteVm, scaler.handleDeleteVm)
	engine.POST(config.RouteWebhook, scaler.handleWebhook)
	engine.GET("/healthcheck", func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
	return &scaler
}

func (s *Autoscaler) Srv(port int) {

	s.engine.Run(fmt.Sprintf("0.0.0.0:%d", port))
}
