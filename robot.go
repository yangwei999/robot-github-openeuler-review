package main

import (
	"fmt"

	sdk "github.com/google/go-github/v36/github"
	"github.com/opensourceways/community-robot-lib/config"
	gc "github.com/opensourceways/community-robot-lib/githubclient"
	framework "github.com/opensourceways/community-robot-lib/robot-github-framework"
	"github.com/opensourceways/community-robot-lib/utils"
	cache "github.com/opensourceways/repo-file-cache/sdk"
	"github.com/sirupsen/logrus"
)

const botName = "review"

type iClient interface {
	AddPRLabel(pr gc.PRInfo, label string) error
	RemovePRLabel(pr gc.PRInfo, label string) error
	CreatePRComment(pr gc.PRInfo, comment string) error
	MergePR(pr gc.PRInfo, commitMessage string, opt *sdk.PullRequestOptions) error
	ListOperationLogs(pr gc.PRInfo) ([]*sdk.Timeline, error)
	GetPathContent(org, repo, path, branch string) (*sdk.RepositoryContent, error)
	GetPullRequestChanges(pr gc.PRInfo) ([]*sdk.CommitFile, error)
	GetPRLabels(pr gc.PRInfo) ([]string, error)
	GetRepositoryLabels(pr gc.PRInfo) ([]string, error)
	ListIssueComments(is gc.PRInfo) ([]*sdk.IssueComment, error)
	CreateRepoLabel(org, repo, label string) error
	GetUserPermissionOfRepo(org, repo, user string) (*sdk.RepositoryPermissionLevel, error)
	GetDirectoryTree(org, repo, branch string, recursive bool) ([]*sdk.TreeEntry, error)
	GetSinglePR(org, repo string, number int) (*sdk.PullRequest, error)
	GetPullRequests(pr gc.PRInfo) ([]*sdk.PullRequest, error)
}

func newRobot(cli iClient, cacheCli *cache.SDK) *robot {
	return &robot{cli: cli, cacheCli: cacheCli}
}

type robot struct {
	cli      iClient
	cacheCli *cache.SDK
}

func (bot *robot) NewConfig() config.Config {
	return &configuration{}
}

func (bot *robot) getConfig(cfg config.Config, org, repo string) (*botConfig, error) {
	c, ok := cfg.(*configuration)
	if !ok {
		return nil, fmt.Errorf("can't convert to configuration")
	}

	if bc := c.configFor(org, repo); bc != nil {
		return bc, nil
	}

	return nil, fmt.Errorf("no config for this repo:%s/%s", org, repo)
}

func (bot *robot) RegisterEventHandler(p framework.HandlerRegister) {
	p.RegisterPullRequestHandler(bot.handlePREvent)
	p.RegisterIssueCommentHandler(bot.handleCommentEvent)
}

func (bot *robot) handlePREvent(e *sdk.PullRequestEvent, pc config.Config, log *logrus.Entry) error {
	org, repo := gc.GetOrgRepo(e.GetRepo())
	cfg, err := bot.getConfig(pc, org, repo)
	if err != nil {
		return err
	}
	pr := gc.PRInfo{Org: org, Repo: repo, Number: e.GetNumber()}

	merr := utils.NewMultiErrors()
	if err := bot.clearLabel(e, pr); err != nil {
		merr.AddError(err)
	}

	if err := bot.doRetest(e, pr); err != nil {
		merr.AddError(err)
	}

	if err := bot.checkReviewer(e, pr, cfg); err != nil {
		merr.AddError(err)
	}

	if err := bot.handleLabelUpdate(e, pr, cfg, log); err != nil {
		merr.AddError(err)
	}

	return merr.Err()
}

func (bot *robot) handleCommentEvent(e *sdk.IssueCommentEvent, pc config.Config, log *logrus.Entry) error {
	org, repo := gc.GetOrgRepo(e.GetRepo())
	cfg, err := bot.getConfig(pc, org, repo)
	if err != nil {
		return err
	}

	merr := utils.NewMultiErrors()
	if err := bot.handleLGTM(e, cfg, log); err != nil {
		merr.AddError(err)
	}

	if err = bot.handleApprove(e, cfg, log); err != nil {
		merr.AddError(err)
	}

	if err = bot.handleCheckPR(e, cfg, log); err != nil {
		merr.AddError(err)
	}

	if err = bot.removeInvalidCLA(e, cfg, log); err != nil {
		merr.AddError(err)
	}

	if err = bot.handleRebase(e, cfg, log); err != nil {
		merr.AddError(err)
	}

	if err = bot.handleFlattened(e, cfg, log); err != nil {
		merr.AddError(err)
	}

	if err = bot.removeRebase(e, cfg, log); err != nil {
		merr.AddError(err)
	}

	if err = bot.removeFlattened(e, cfg, log); err != nil {
		merr.AddError(err)
	}

	if err = bot.handleACK(e, cfg, log); err != nil {
		merr.AddError(err)
	}

	return merr.Err()
}
