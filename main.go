package main

import (
	"flag"
	"net/url"
	"os"

	cache "github.com/opensourceways/repo-file-cache/sdk"
	"github.com/opensourceways/robot-github-lib/client"
	"github.com/opensourceways/robot-github-lib/framework"
	"github.com/opensourceways/server-common-lib/logrusutil"
	liboptions "github.com/opensourceways/server-common-lib/options"
	"github.com/opensourceways/server-common-lib/secret"
	"github.com/sirupsen/logrus"
)

type options struct {
	service       liboptions.ServiceOptions
	github        liboptions.GithubOptions
	cacheEndpoint string
	maxRetries    int
}

func (o *options) Validate() error {
	if _, err := url.ParseRequestURI(o.cacheEndpoint); err != nil {
		return err
	}

	if err := o.service.Validate(); err != nil {
		return err
	}

	return o.github.Validate()
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options

	o.github.AddFlags(fs)
	o.service.AddFlags(fs)
	fs.StringVar(&o.cacheEndpoint, "cache-endpoint", "", "The endpoint of repo file cache")
	fs.IntVar(&o.maxRetries, "max-retries", 3, "The number of failed retry attempts to call the cache api")

	_ = fs.Parse(args)

	return o
}

func main() {
	logrusutil.ComponentInit(botName)

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	secretAgent := new(secret.Agent)
	if err := secretAgent.Start([]string{o.github.TokenPath}); err != nil {
		logrus.WithError(err).Fatal("Error starting secret agent.")
	}

	defer secretAgent.Stop()

	c := client.NewClient(secretAgent.GetTokenGenerator(o.github.TokenPath))
	s := cache.NewSDK(o.cacheEndpoint, o.maxRetries)

	p := newRobot(c, s)

	framework.Run(p, o.service)
}
