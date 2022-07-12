package main

import (
	"encoding/base64"
	gc "github.com/opensourceways/community-robot-lib/githubclient"
	"path/filepath"
	"strings"

	sdk "github.com/google/go-github/v36/github"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"
)

const ownerFile = "OWNERS"
const sigInfoFile = "sig-info.yaml"

func (bot *robot) hasPermission(
	org, repo, commenter string,
	needCheckSig bool,
	e *sdk.IssueCommentEvent,
	cfg *botConfig,
	log *logrus.Entry,
) (bool, error) {
	commenter = strings.ToLower(commenter)
	p, err := bot.cli.GetUserPermissionOfRepo(org, repo, commenter)
	if err != nil {
		return false, err
	}

	if *p.Permission == "admin" || *p.Permission == "write" {
		return true, nil
	}

	if needCheckSig {
		return bot.isOwnerOfSig(org, repo, commenter, e, cfg, log)
	}

	return false, nil
}

func (bot *robot) isOwnerOfSig(
	org, repo, commenter string,
	e *sdk.IssueCommentEvent,
	cfg *botConfig,
	log *logrus.Entry,
) (bool, error) {
	changes, err := bot.cli.GetPullRequestChanges(gc.PRInfo{Org: org, Repo: repo, Number: e.GetIssue().GetNumber()})
	if err != nil || len(changes) == 0 {
		return false, err
	}

	pathes := sets.NewString()
	for _, file := range changes {
		if !cfg.regSigDir.MatchString(*file.Filename) || strings.Count(*file.Filename, "/") > 2 {
			return false, nil
		}

		pathes.Insert(filepath.Dir(*file.Filename))
	}

	// get directory tree
	oPath, sPath, err := bot.listDirectoryTree(org, repo, "master", cfg.SigsDir)
	if err != nil {
		return false, nil
	}

	for _, o := range oPath {
		p := filepath.Dir(o)
		if !pathes.Has(p) {
			continue
		}

		oFile, err := bot.cli.GetPathContent(org, repo, o, "master")
		if err != nil || oFile == nil {
			return false, nil
		}

		if o := decodeOwnerFile(*oFile.Content, log); !o.Has(commenter) {
			return false, nil
		}

		pathes.Delete(p)

		if len(pathes) == 0 {
			return true, nil
		}
	}

	for _, s := range sPath {
		p := filepath.Dir(s)
		if !pathes.Has(p) {
			continue
		}

		sFile, err := bot.cli.GetPathContent(org, repo, s, "master")
		if err != nil || sFile == nil {
			return false, nil
		}

		if o := decodeSigInfoFile(*sFile.Content, log); !o.Has(commenter) {
			return false, nil
		}

		pathes.Delete(p)

		if len(pathes) == 0 {
			return true, nil
		}
	}

	return false, nil
}

func (bot *robot) listDirectoryTree(org, repo, branch, dirPath string) ([]string, []string, error) {
	recursive := true
	ownerFilePath := make([]string, 0)
	sigInfoFilePath := make([]string, 0)
	trees, err := bot.cli.GetDirectoryTree(org, repo, branch, recursive)
	if err != nil {
		return nil, nil, err
	}

	for _, t := range trees {
		if !strings.Contains(*t.Path, dirPath) {
			continue
		}
		if strings.Count(*t.Path, "/") == 2 && strings.Contains(*t.Path, ownerFile) {
			ownerFilePath = append(ownerFilePath, *t.Path)
		}

		if strings.Count(*t.Path, "/") == 2 && strings.Contains(*t.Path, sigInfoFile) {
			sigInfoFilePath = append(sigInfoFilePath, *t.Path)
		}
	}

	return ownerFilePath, sigInfoFilePath, nil
}

func decodeSigInfoFile(content string, log *logrus.Entry) sets.String {
	owners := sets.NewString()

	c, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		log.WithError(err).Error("decode file")

		return owners
	}

	var m SigInfos

	if err = yaml.Unmarshal(c, &m); err != nil {
		log.WithError(err).Error("code yaml file")

		return owners
	}

	for _, v := range m.Maintainers {
		owners.Insert(strings.ToLower(v.GiteeID))
	}

	return owners
}

func decodeOwnerFile(content string, log *logrus.Entry) sets.String {
	owners := sets.NewString()

	c, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		log.WithError(err).Error("decode file")

		return owners
	}

	var m struct {
		Maintainers []string `yaml:"maintainers"`
		Committers  []string `yaml:"committers"`
	}

	if err = yaml.Unmarshal(c, &m); err != nil {
		log.WithError(err).Error("code yaml file")

		return owners
	}

	for _, v := range m.Maintainers {
		owners.Insert(strings.ToLower(v))
	}

	for _, v := range m.Committers {
		owners.Insert(strings.ToLower(v))
	}

	return owners
}
