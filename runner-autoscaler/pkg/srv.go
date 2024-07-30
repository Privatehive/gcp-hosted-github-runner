package pkg

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	ginlogrus "github.com/toorop/gin-logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const SHA_PREFIX = "sha256="
const SHA_HEADER = "x-hub-signature-256"
const EVENT_HEADER = "x-github-event"

const WEBHOOK_PING_EVENT = "ping"
const WEBHOOK_JOB_EVENT = "workflow_job"

type Job struct {
	Id              int64    `json:"id"`
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	Labels          []string `json:"labels"`
	RunnerName      string   `json:"runner_name"`
	RunnerGroupName string   `json:"runner_group_name"`
}

type Payload struct {
	Action Action `json:"action"`
	Job    Job    `json:"workflow_job"`
}

func (j Job) hasLabel(label string) bool {

	for _, l := range j.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// returns true if all labels were found. false otherwise. Returns also all labels that were missing
func (j Job) hasAllLabels(labels []string) (bool, []string) {

	missingLabels := []string{}
	for _, label := range labels {
		if !j.hasLabel(label) {
			missingLabels = append(missingLabels, label)
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

func (s State) isStopped() bool {

	return s == STOPPING || s == SUSPENDING || s == SUSPENDED || s == TERMINATED
}

func (s State) isRunning() bool {

	return s == PROVISIONING || s == STAGING || s == RUNNING || s == REPAIRING
}

type InstanceClient struct {
	*compute.InstancesClient
}

func createCallbackUrl(ctx *gin.Context, path string) string {

	return "https://" + ctx.Request.Host + path
}

func newComputeClient() *InstanceClient {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if client, err := compute.NewInstancesRESTClient(ctx); err != nil {
		panic(err)
	} else {
		return &InstanceClient{client}
	}
}

func newTaskClient() *cloudtasks.Client {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if client, err := cloudtasks.NewClient(ctx); err != nil {
		panic(err)
	} else {
		return client
	}
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")

func randStringRunes(n int) string {

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

func (s *Autoscaler) verifySignature(ctx *gin.Context) ([]byte, error) {

	if signature := ctx.GetHeader(SHA_HEADER); len(signature) == 71 {
		if body, err := io.ReadAll(ctx.Request.Body); err != nil {
			log.Errorf("Error receiving http body: %s", err.Error())
			ctx.AbortWithError(http.StatusBadRequest, err)
			return nil, err
		} else {
			if calcSignature := calcSigHex([]byte(s.conf.WebhookSecret), body); calcSignature == signature[7:] {
				return body, nil
			}
		}
	}

	log.Warnf("%s is unauthorized", ctx.RemoteIP())
	ctx.AbortWithStatus(http.StatusUnauthorized)
	return nil, fmt.Errorf("unauthorized")
}

func (s *Autoscaler) getInstanceState(ctx context.Context, instanceName string) (State, error) {

	if res, err := s.c.Get(ctx, &computepb.GetInstanceRequest{
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
func (s *Autoscaler) startInstance(ctx context.Context, instanceName string) error {

	log.Infof("About to start instance: %s", instanceName)
	if res, err := s.c.Start(ctx, &computepb.StartInstanceRequest{
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
func (s *Autoscaler) stopInstance(ctx context.Context, instanceName string) error {

	log.Infof("About to stop instance: %s", instanceName)
	if res, err := s.c.Stop(ctx, &computepb.StopInstanceRequest{
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
func (s *Autoscaler) deleteInstance(ctx context.Context, instanceName string) error {

	log.Infof("About to delete instance: %s", instanceName)
	if res, err := s.c.Delete(ctx, &computepb.DeleteInstanceRequest{
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
func (s *Autoscaler) createInstanceFromTemplate(ctx context.Context, instanceName string) error {

	if res, err := s.c.Insert(ctx, &computepb.InsertInstanceRequest{
		Project: s.conf.ProjectId,
		Zone:    s.conf.Zone,
		InstanceResource: &computepb.Instance{
			Name: proto.String(instanceName),
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

func (s *Autoscaler) createCallbackTaskWithToken(ctx context.Context, url, message string) (*taskspb.Task, error) {

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
						SHA_HEADER: SHA_PREFIX + calcSigHex([]byte(s.conf.WebhookSecret), []byte(message)),
					},
				},
			},
		},
	}

	req.Task.GetHttpRequest().Body = []byte(message)

	createdTask, err := s.t.CreateTask(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("cloudtasks.CreateTask failed: %v", err)
	} else {
		log.Infof("Created cloud task callback with url \"%s\" and payload \"%s\"", url, message)
	}

	return createdTask, nil
}

func (s *Autoscaler) handleCreateVm(ctx *gin.Context) {

	log.Info("Received create-vm cloud task callback")
	if data, err := s.verifySignature(ctx); err == nil {
		if err := s.createInstanceFromTemplate(ctx, string(data)); err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
		} else {
			ctx.Status(http.StatusOK)
		}
	}
}

func (s *Autoscaler) handleDeleteVm(ctx *gin.Context) {

	log.Info("Received delete-vm cloud task callback")
	if data, err := s.verifySignature(ctx); err == nil {
		if err := s.deleteInstance(ctx, string(data)); err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
		} else {
			ctx.Status(http.StatusOK)
		}
	}
}

func (s *Autoscaler) handleWebhook(ctx *gin.Context) {

	log.Info("Received webhook")
	if data, err := s.verifySignature(ctx); err == nil {
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
					if ok, missingLabels := payload.Job.hasAllLabels(s.conf.RunnerLabels); ok {
						createUrl := createCallbackUrl(ctx, s.conf.RouteCreateVm)
						if _, err := s.createCallbackTaskWithToken(ctx, createUrl, fmt.Sprintf("%s-%s", s.conf.RunnerPrefix, randStringRunes(10))); err != nil {
							log.Errorf("Can not enqueue create-vm cloud task callback: %s", err.Error())
							ctx.AbortWithError(http.StatusInternalServerError, err)
							return
						}
					} else {
						log.Warnf("Webhook requested to start a runner that is missing the label(s) \"%s\" - ignoring", strings.Join(missingLabels, ", "))
					}
				} else if payload.Action == COMPLETED {
					if payload.Job.RunnerGroupName == s.conf.RunnerGroup {
						if ok, missingLabels := payload.Job.hasAllLabels(s.conf.RunnerLabels); ok {
							deleteUrl := createCallbackUrl(ctx, s.conf.RouteDeleteVm)
							if _, err := s.createCallbackTaskWithToken(ctx, deleteUrl, payload.Job.RunnerName); err != nil {
								log.Errorf("Can not enqueue delete-vm cloud task callback: %s", err.Error())
								ctx.AbortWithError(http.StatusInternalServerError, err)
								return
							}
						} else {
							log.Warnf("Webhook signaled to delete a runner that is missing the label(s) \"%s\" - ignoring", strings.Join(missingLabels, ", "))
						}
					} else {
						log.Warnf("Webhook signaled to delete a runner that does not belong to the expected runner group (expected \"%s\" got \"%s\") - ignoring", s.conf.RunnerGroup, payload.Job.RunnerGroupName)
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

type AutoscalerConfig struct {
	RouteWebhook     string
	RouteCreateVm    string
	RouteDeleteVm    string
	WebhookSecret    string
	ProjectId        string
	Zone             string
	TaskQueue        string
	InstanceTemplate string
	RunnerPrefix     string
	RunnerGroup      string
	RunnerLabels     []string
}

type Autoscaler struct {
	engine *gin.Engine
	c      *InstanceClient
	t      *cloudtasks.Client
	conf   AutoscalerConfig
}

func NewAutoscaler(config AutoscalerConfig) Autoscaler {

	engine := gin.New()
	scaler := Autoscaler{
		engine: engine,
		c:      newComputeClient(),
		t:      newTaskClient(),
		conf:   config,
	}
	engine.Use(ginlogrus.Logger(log.WithFields(log.Fields{})))
	engine.POST(config.RouteCreateVm, scaler.handleCreateVm)
	engine.POST(config.RouteDeleteVm, scaler.handleDeleteVm)
	engine.POST(config.RouteWebhook, scaler.handleWebhook)
	engine.GET("/healthcheck", func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
	return scaler
}

func (s *Autoscaler) Srv(port int) {

	s.engine.Run(fmt.Sprintf("0.0.0.0:%d", port))
}
