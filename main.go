package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/retry"
)

type config struct {
	PrivateToken   string `env:"private_token,required"`
	ProjectID      string `env:"gitlab_project_id,required"`
	CommitHash     string `env:"commit_hash,required"`
	PipelineID     string `env:"gitlab_pipeline_id,required"`
	APIURL         string `env:"api_base_url,required"`

	Status      string  `env:"preset_status,opt[auto,pending,running,success,failed,canceled]"`
	TargetURL   string  `env:"target_url"`
	Context     string  `env:"context"`
	Description string  `env:"description"`
	Coverage    float64 `env:"coverage,range[0.0..100.0]"`
}

func getState(preset string) string {
	if preset != "auto" {
		return preset
	}
	if os.Getenv("BITRISE_BUILD_STATUS") == "0" {
		return "success"
	}
	return "failed"
}

func getDescription(desc, state string) string {
	if desc == "" {
		return strings.Title(getState(state))
	}
	return desc
}

// sendStatus creates a commit status for the given commit.
// see also: https://docs.gitlab.com/ce/api/commits.html#post-the-build-status-to-a-commit
func sendStatus(cfg config) error {
	repo := cfg.ProjectID
	form := url.Values{
		"state":       {getState(cfg.Status)},
		"target_url":  {cfg.TargetURL},
		"description": {getDescription(cfg.Description, cfg.Status)},
		"context":     {cfg.Context},
		"coverage":    {fmt.Sprintf("%f", cfg.Coverage)},
		"pipeline_id": {cfg.PipelineID},
	}

	url := fmt.Sprintf("%s/projects/%s/statuses/%s", cfg.APIURL, repo, cfg.CommitHash)
	req, err := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Add("PRIVATE-TOKEN", cfg.PrivateToken)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send the request: %s", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := resp.Body.Close(); err != nil {
		return err
	}
	if 200 > resp.StatusCode || resp.StatusCode >= 300 {
		return fmt.Errorf("server error: %s url: %s code: %d body: %s", resp.Status, url, resp.StatusCode, string(body))
	}

	return err
}

func main() {
	if os.Getenv("commit_hash") == "" {
		log.Warnf("GitLab requires a commit hash for build status reporting")
		os.Exit(1)
	}

	var cfg config
	if err := stepconf.Parse(&cfg); err != nil {
		log.Errorf("Error: %s\n", err)
		os.Exit(1)
	}
	stepconf.Print(cfg)

	if err := retry.Times(3).Wait(5 * time.Second).Try(func(attempt uint) error {
		if attempt > 0 {
			log.Warnf("%d attempt failed", attempt)
		}

		return sendStatus(cfg)
	}); err != nil {
		log.Errorf("Failed to set status, error: %s", err)
		os.Exit(1)
	}
}
