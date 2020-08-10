package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/michaeloverton/waterbot/internal/env"
	"github.com/michaeloverton/waterbot/internal/esp"
	"github.com/michaeloverton/waterbot/internal/throttle"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Set up logger output for EC2 storage.
	// log.New().SetOutput()

	// Load environment vars.
	env, err := env.LoadEnv()
	if err != nil {
		log.Fatal(err)
	}

	// Set up ESP2866 client.
	espClient := esp.NewClient(env.EspBaseURL, nil)

	// Create Twitter client.
	config := oauth1.NewConfig(env.Twitter.ApiKey, env.Twitter.ApiSecret)
	token := oauth1.NewToken(env.Twitter.AccessToken, env.Twitter.AccessTokenSecret)
	httpClient := config.Client(oauth1.NoContext, token)
	twitterClient := twitter.NewClient(httpClient)

	// Convenience Demux demultiplexed stream messages
	demux := twitter.NewSwitchDemux()
	demux.Tweet = func(tweet *twitter.Tweet) {
		log.Info(tweet.Text)

		screenName := tweet.User.ScreenName

		if strings.Contains(tweet.Text, "blast the cache") {
			log.Infof("%s reset the cache", screenName)
			throttle.ResetCache()
			return
		}

		// If user can water, do it and say so.
		if throttle.UserCanWater(screenName) {
			// Water the plants.
			if err := espClient.Water(context.Background()); err != nil {
				log.Error(errors.Wrap(err, "failed to water plants"))
				if err := reply(twitterClient, tweet, "i failed"); err != nil {
					log.Error(errors.Wrap(err, "failed to reply upon watering failure"))
				}
				return
			}

			// Indicate watering success.
			if err := reply(twitterClient, tweet, "thirst quenched"); err != nil {
				log.Error(errors.Wrap(err, "failed to reply upon watering success"))
				return
			}

			// Update the user cache.
			throttle.UpdateCache(screenName)

			return
		}

		log.Infof("throttled user: %s. total waters: %d", screenName, throttle.GetUser(screenName).TotalWaters)

		// Otherwise, yell at user.
		if err := reply(twitterClient, tweet, "who do you think you are? you've had your fun"); err != nil {
			log.Error(errors.Wrap(err, "failed to reply upon watering disallow"))
			return
		}
	}

	log.Info("listening for tweets")

	// Set up stream.
	stream, err := twitterClient.Streams.Filter(&twitter.StreamFilterParams{
		Track: []string{"@thirstyplantss"},
		// Follow:        []string{"halloumi_mane", "766041800"},
		StallWarnings: twitter.Bool(true),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Receive messages until stopped or stream quits
	go demux.HandleChan(stream.Messages)

	// Wait for SIGINT and SIGTERM (HIT CTRL-C)
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Println(<-ch)

	log.Info("bye")
	stream.Stop()
}

func reply(twitterClient *twitter.Client, tweet *twitter.Tweet, reply string) error {
	// Don't let bot reply to itself.
	if tweet.User.ScreenName == "thirstyplantss" {
		return nil
	}

	_, _, err := twitterClient.Statuses.Update(
		fmt.Sprintf("@%s %s", tweet.User.ScreenName, reply),
		&twitter.StatusUpdateParams{
			InReplyToStatusID: tweet.ID,
		},
	)
	if err != nil {
		return err
	}

	return nil
}
