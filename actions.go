package main

import (
	"fmt"
	gc "github.com/opensourceways/community-robot-lib/githubclient"
	"k8s.io/apimachinery/pkg/util/sets"
	"strings"

	sdk "github.com/google/go-github/v36/github"
)

const (
	sourceBranchChanged = "synchronize"
	open                = "open"
	retestCommand       = "/retest"
	updateLabel         = "labeled"
	msgNotSetReviewer   = "**@%s** Thank you for submitting a PullRequest. It is detected that you have not set a reviewer, please set a one."
)

func (bot *robot) doRetest(e *sdk.PullRequestEvent, p gc.PRInfo) error {
	if e.GetAction() != sourceBranchChanged || e.GetPullRequest().GetState() != open {
		return nil
	}

	return bot.cli.CreatePRComment(p, retestCommand)
}

func (bot *robot) checkReviewer(e *sdk.PullRequestEvent, p gc.PRInfo, cfg *botConfig) error {
	if cfg.UnableCheckingReviewerForPR || e.GetPullRequest().GetState() != open {
		return nil
	}

	if e.GetPullRequest() != nil && len(e.GetPullRequest().Assignees) > 0 {
		return nil
	}

	return bot.cli.CreatePRComment(p, fmt.Sprintf(msgNotSetReviewer, e.GetPullRequest().GetUser().GetLogin()))
}

func (bot *robot) clearLabel(e *sdk.PullRequestEvent, p gc.PRInfo) error {
	if e.GetAction() != sourceBranchChanged || e.GetPullRequest().GetState() != open {
		return nil
	}

	labels := sets.NewString()
	for _, l := range e.GetPullRequest().Labels {
		labels.Insert(*l.Name)
	}
	v := getLGTMLabelsOnPR(labels)

	if labels.Has(approvedLabel) {
		v = append(v, approvedLabel)
	}

	if len(v) > 0 {
		for _, vv := range v {
			if err := bot.cli.RemovePRLabel(p, vv); err != nil {
				return err
			}
		}

		return bot.cli.CreatePRComment(p, fmt.Sprintf(commentClearLabel, strings.Join(v, ", ")))
	}

	return nil
}
