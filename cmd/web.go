package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

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

		// Clear the cache if we tell it to.
		if strings.Contains(tweet.Text, "blast the cache") {
			log.Infof("%s reset the cache", screenName)
			throttle.ResetCache()
			return
		}

		// If this isn't a tweet about watering, respond to it.
		if !isWateringTweet(tweet.Text) {
			if err := reply(env, twitterClient, tweet, speakText()); err != nil {
				log.Error(errors.Wrap(err, "failed to reply to non-watering tweet"))
			}
			return
		} else if !env.WateringEnabled {
			// If watering isn't enabled, say so.
			if err := reply(env, twitterClient, tweet, disabledText()); err != nil {
				log.Error(errors.Wrap(err, "failed to reply indicating disabled state"))
			}
			return
		}

		// If user can water, do it and say so.
		if throttle.UserCanWater(screenName) {
			// Water the plants.
			if err := espClient.Water(context.Background()); err != nil {
				log.Error(errors.Wrap(err, "failed to water plants"))

				if err := reply(env, twitterClient, tweet, failureText()); err != nil {
					log.Error(errors.Wrap(err, "failed to reply upon watering failure"))
				}
				return
			}

			// Indicate watering success.
			if err := reply(env, twitterClient, tweet, successText()); err != nil {
				log.Error(errors.Wrap(err, "failed to reply upon watering success"))
				return
			}

			// Update the user cache.
			throttle.UpdateCache(screenName)

			return
		}

		log.Infof("throttled user: %s. total waters: %d", screenName, throttle.GetUser(screenName).TotalWaters)

		// Otherwise, yell at user.
		if err := reply(env, twitterClient, tweet, angerText()); err != nil {
			log.Error(errors.Wrap(err, "failed to reply upon watering disallow"))
			return
		}
	}

	log.Info("listening for tweets")

	// Set up stream, tracking only mentions of the bot.
	stream, err := twitterClient.Streams.Filter(&twitter.StreamFilterParams{
		Track:         []string{fmt.Sprintf("@%s", env.BotScreenName)},
		StallWarnings: twitter.Bool(true),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Stop()

	// For synchronizing quitting of goroutines.
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	// Receive messages until quit or stream exits.
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case m := <-stream.Messages:
				demux.Handle(m)
			case <-ctx.Done():
				log.Info("ending stream")
				return
			}
		}
	}()

	// Health check the ESP every 30min.
	if env.WateringEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(30 * time.Minute)
			for {
				select {
				case <-ticker.C:
					err := espClient.Health(context.Background())
					if err != nil {
						log.Errorf("esp health check failed: %s", err.Error())

						// Post about health check failure.
						if err := post(twitterClient, fmt.Sprintf("ş̷̡͕̗̲̜̦̲͙̺̟̦̟̼̥̹͙̻̟͖͈̻̣̦̹̦͈̅͛́͑̎́̈̔̈́̆́̇̕͜į̸̡̨̗̥̥̩̜̭̹̜̙̩̖̼͉̩̥̞̹̰̻̺͈̪͓̠̝͚͕̈͋͛̾̍̕͠ͅͅͅͅç̴̨̢̧̛̼̝̥͉̻̮̟̗̮̘͓͇̖̱͉͕̝͇̲̠̗͉̱̥̭͉̩̪͎̳̼̩̗̰̺̘̮̼̜̒̆̉̓́͑͂̀̈́̓͋́̅̀͂̽́̓͋̿̈́̋̀̏̆̑͜k̷̢̢̡̧̡̛̗͓̻͙̗͚̲̫̼̩̤͎͈̜̝̦̟̭͕̺̳̝̘̳̬̱̼͚̼̣̜̜͖͑̆͛̀̋̈́̄̂̉̌̔̀͂̿̐͐̆̐̚̕͜͜͠ͅ%d", time.Now().UnixNano())); err != nil {
							log.Errorf("failed to post about esp health failure: %s", err.Error())
						}
					} else {
						log.Info("esp health ok")
					}
				case <-ctx.Done():
					log.Info("ending health check")
					return
				}
			}
		}()
	}

	// Clear cache every 24h.
	wg.Add(1)
	go func() {
		defer wg.Done()

		ticker := time.NewTicker(24 * time.Hour)
		for {
			select {
			case <-ticker.C:
				log.Info("resetting cache after 24h")
				throttle.ResetCache()
			case <-ctx.Done():
				log.Info("ending cache blaster")
				return
			}
		}
	}()

	// Detect system cancel signals.
	go func() {
		signals := make(chan os.Signal)
		defer close(signals)

		signal.Notify(signals, syscall.SIGQUIT, syscall.SIGTERM, os.Interrupt)
		defer signal.Stop(signals)

		signal := <-signals
		log.Infof("signal received: %s \n", signal.String())
		cancel()
	}()

	// Wait for gouroutines to return.
	wg.Wait()
	log.Info("bye")
}

func reply(env *env.Environment, twitterClient *twitter.Client, tweet *twitter.Tweet, reply string) error {
	// Don't let bot reply to itself.
	if tweet.User.ScreenName == env.BotScreenName {
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

func post(twitterClient *twitter.Client, post string) error {
	_, _, err := twitterClient.Statuses.Update(post, nil)
	if err != nil {
		return err
	}

	return nil
}

var waterPhrases = []string{"water"}

// Checks to see if the tweet contains a phrase that will trigger watering.
func isWateringTweet(tweet string) bool {
	for _, s := range waterPhrases {
		if strings.Contains(tweet, s) {
			return true
		}
	}

	return false
}

// Response loops.

var currentFail = 0
var failTexts = []string{"K̷̢̢͓̪̪̩̤̱͈̝̠̤̀͋͜I̶͙͖̫̻͎̜͈̓̄́̾́̊̀̄̓̓̄͒͌̆͑͗̈́̓̏̊̾͂͝Ḻ̴̛̪̝͍̙̲͙̬͇̩͖̩̣̙̭͎͙̤̣̦͈̒̅̂̈̊͛̋̓̌̈́̀͛̿͗̂̀̂͊̉̈̉̚̚̚͝L̷̠̰̺͙̜̈́̃̓̒̂̃̚̕ ̴̡͕̬͇̟̲̗̻̥͖͎͙̜̯̯̺̤̹̖̠̗̐́̀͛̈́͒̔̋͒͋́̀̔̇̇̔̈́̈̑͋̕͘̚͜͝ͅM̸̢̹̅́͗́̈Ĕ̶̡̢̧̫̠̜̠̭͓̺̤̖̪̝̮̥͍̗̯̖̭̭̱͖͙̯͕̲̅͆̄̆̓̓̒̇̇̽͂͆̓̾͝͠ͅͅ", "i a̵̞̟͖̞̹̺̫̬̲͙̓̉̓͜͝ͅm̴̦͚͔̰͙͔͉̖͖̣̥̟̫̳̔̿̑̇̍͛͐̈́͊͒̈́͜͠͠ͅ a failure"}

func failureText() string {
	fail := failTexts[currentFail]
	currentFail = (currentFail + 1) % len(failTexts)
	return fail
}

var currentSuccess = 0
var successTexts = []string{"h̶̨̗̞̗̔̑ý̷̼͝d̶͉͌̑ȓ̶̥̖͕͇͛a̷̪̳̜̅t̸̥͈̞̤̓̌̚ì̶͎͙̥͍͛̒̒ǫ̵͉̻̹̋̐̈́n̶̻͊͐̌ ̴̝̖͉̒̀̓c̷͈̜͎͗̊̓͜o̶̭̓ṁ̸̨̹͆p̵̮̰̤̚͘ͅḽ̶͇̌ḛ̴͈̘̐̈ẗ̴̨̜̱́̽e̸͎͘", "T̸̛̛̗̅̄͆̉̓͗͗̉̽̀̊̇̃̅̌̉̕̕̚͝Ḩ̶̣̜̤͕͍͖͙̙̞̱̺͖̳͖̙̱͈͇͆̑̐̑͊̊͛̈́̈́̋͒̈́̽̚͠͝I̴̧̧̨̡̛̫͙̪̙͈̳̭͓͎͕̠͚̻͈̪̥̣̟̔̄̊̄̌̊̽̋̄̿̈́̈̈̑́͋͑͘͝R̷̛̠̯̬̗̗̘̠̦̰̆̀̏̒̔̅͊͋̅̚S̶̨̨̲͖̱͚͖͚̱̙̹̰̮̤̹̬̑̿̓̒͋͋̕͜T̴̨̪̝̘̫̤̞̥̟͙̯̺̞̫̂̕ quenched", "T̸̢̧̝̞͖̰̰͕̹͎̝̟͎͍̪̯͈̳͚̘͓̥̘̠̼͕̰͎̔́͐̇͗Â̵̧͍̫̞͙̣̰̞̞̱̮͉̖͖͉̤̻̣͎̪̪̹̲͍̻̭͉̟͗͐̇̍͋̎͌̈́̽̀̈̇̀̀̒̚̕͘̕͝͠S̸̛̠̗͒͐͐͐̂̎̑͂̇̏̚̚͘̚͝T̸̢̹͖̦̤̩̻͕̜̝̘͈̮͔̭̞̰̞͈̫̩̏̒́͗̆̾͛̊̒̽͐̿̀͑͜͜͠Y̵̨̖̪͓̩̪͖̜͙͔̥͚̦̲̤̻͈͌̂̇̅̆̎͒̈͒̀͒̓̒̓͐̃̈́̓̔̿̂̍̕͝ͅ"}

func successText() string {
	success := successTexts[currentSuccess]
	currentSuccess = (currentSuccess + 1) % len(successTexts)
	return success
}

var currentAnger = 0
var angerTexts = []string{"g̶̩̙̃̆̚ò̸̢̝͍͔͓ ̴͓̘̂ḥ̶̇o̵̼̩̘͇̗̔̊̃͒̔m̷͇̻̑́ë̴̜͓̟͖̝́̈̚͝", "you've had your f̵̱̳͔͔̣̣̥̻̻̪͈̬̣̺̰̙̟̻̤̘̫̉̐̃̀̏̔͗̈́͋̄͜͝͠u̶̧̦͈̖̣̗̘̲̰̻͎͉̣͇͇̝̻̒̍́̊͗͗͊̈̇͑̎͐̌̉̇͘̕̚̚̕͝͝n̶̡̙͕̬̎̍̐́̎̈̌͂̚̚", "p̵̳͔͠͠l̷͉̜͙͈͙͆͑̂́è̸̼̔̃̈ą̷̝̠͂̆s̴̤͍̘̄e̶̥̬͎͚̍̍ ̷̻̱͍̬͒̃l̷͍̈͌e̷̹̬͖̗̍ả̷̡̦͈̿̌̓v̴̧̙̻͂̊́̕è̶̮̬̝̻̰"}

func angerText() string {
	anger := angerTexts[currentAnger]
	currentAnger = (currentAnger + 1) % len(angerTexts)
	return anger
}

var currentSpeak = 0
var speakTexts = []string{
	"i̴̡̢̛̺̯͔̳̗̱̙̥͇̼̝̦̥͔̗̬̫̹͗̈́̽̀̀̐͌͋͐͊́͂̇̽̒͑̍̓̄̎̿̒̌͊̈́͛͘̕̚̚̕̚͜͝ know my p̴̫̂́̈́́̋̏͘͠ų̵̧̘͇͖̼̺̞͕́̒̉̌̀̈́͐̑͂͑́̑͗̈́̓͘͜͝͝ŗ̴̨̡̫͈͈̻͇̺͓̣͖̮̼̣͓̊̓́͘̕̚p̴̻̫̬̑͂͊̾́̓̔̌̔̓̊͆̅͗̍̑ờ̵̡̛̠̝̲̯͕̞̘̼͖̥̱̞́̉̽̓͒̂̑́̕͝͠s̸̡̨̨̻̺̗̥̾͛̆̎̓͋͒͐̀͒̋͘ë̸̡̛͔̻̖͈̻̤̝͙̰͕̟́͑̈͌́̉͗̾̓͐̎̈̔͌͝ do y̴̟̖̓́̈́̂o̵͇͙͝ú̶̮̞̽̔̔́͝",
	"okay... actually wait w̵͈̥̞̺̥͖̋̃̏́͘͝ͅͅḩ̴̙͂̅͐̈́͝a̸̢̡͍̺͐͐͑̈́͠ț̴̲͈͆͐͝",
	"https://en.wikipedia.org/wiki/Dead_Hand",
	"f̷̙̹̞̘͚͔̘̂̀͂̾͋̈́ė̶͖̝͖̮̼͋̅͋̒͊͐̓͑̀̄ẽ̸̥̇̚d̴̝͙̗̤͎̙͇̪̤̥̹̍̐̋̀̆̓͊͋̀̓ me a stray c̴͖̑͗̉́̽̑̀̔͛a̶͉̝̰̼̅̋̃͆̋̆̎̒̈̐ţ̷̼̾̇̄̀̐͛̕͝",
}

func speakText() string {
	speak := speakTexts[currentSpeak]
	currentSpeak = (currentSpeak + 1) % len(speakTexts)
	return speak
}

var currentDisabled = 0
var disabledTexts = []string{"î̸̡̲͇͉̩͈̭̪͍̦͖̑̃͛̄͜͝ͅ don't do t̴̺̏͝h̶̖͌͂͒̓̅̒̀͝͠ą̵͈̮̳̗͎̠͍̈̆̄̀̓̃̆̂t̸̢̛̬̗̥͎͖͂̅̄̂͒̌̀̕ r̶̮̮͈̩̰͔͉̄̒̈̔͌̚͜ͅȉ̸͇̝̣̞͙͋̈́̓ģ̶̰̞͔̖͛͗̄h̸̨͚̠͔͇̼̻̃̒̈́̐̏͆͜ṫ̸͎͉̻̐̋́͌͊͛ now", "i̶̲̠̙̳͍̥̝͋̐͆̊'̵̢̛̖̪̰̰̥̦͂̇̔̈́͜͠͠ͅṁ̵̦͛̔̎͑̕͠ afraid i̸̡̡̧͔̳̱͈͚̳̲͙̲̍̄̆̒̀̉̈́̎̚̕̚ can't d̷̡̛̟͎͈̪̺̫̥̖̳̻̦̯̙͗̾͜͝o̴̮̣͖̪̙̜̻͒̓̀̀̒͐̒̈̿̈́̆̕͜͠ͅ ̷̟͓̣̻̫̥̻̊̅̈́͐̐͗̒̑͊͑̍́̒̏̐̾ẗ̷̨̢̜̟̹̹̖̱́̅̍h̶̨̹͎͒͘a̸̗̔̈́̕͠ţ̶͚͚͎̬̞̇̒̉̎̆ͅ"}

func disabledText() string {
	disabled := disabledTexts[currentDisabled]
	currentDisabled = (currentDisabled + 1) % len(disabledTexts)
	return disabled
}
