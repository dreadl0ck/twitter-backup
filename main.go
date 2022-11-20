/*
 * TWITTER-BACKUP
 * Copyright (c) 2017-2020 Philipp Mieden <dreadl0ck [at] protonmail [dot] ch>
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/coreos/pkg/flagutil"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/dustin/go-humanize"
)

const (
	pathPermission = 0700
	filePermission = 0600
	pathLikes      = "likes"
	pathFollowing  = "following"
)

// simple backup tool to save the raw JSON objects for liked tweets and followed users to the file system,
// as well as the media files for each tweet.
// uses twitter v1.1 api, and needs auth credentials obtained from their dev portal for your account.
func main() {

	var (
		flags = flag.NewFlagSet("auth", flag.ExitOnError)

		// get them from: https://developer.twitter.com/en/portal/dashboard
		// and export in your bashrc / zshrc like this:
		//     export TWITTER_CONSUMER_KEY="XXXXXXXXXXXXX"
		//     export TWITTER_CONSUMER_SECRET="XXXXXXXXXXXXX"
		//     export TWITTER_ACCESS_TOKEN="XXXXXXXXXXXXX"
		//     export TWITTER_ACCESS_SECRET="XXXXXXXXXXXXX"

		consumerKey    = flags.String("consumer-key", "", "Twitter Consumer Key")
		consumerSecret = flags.String("consumer-secret", "", "Twitter Consumer Secret")
		accessToken    = flags.String("access-token", "", "Twitter Access Token")
		accessSecret   = flags.String("access-secret", "", "Twitter Access Secret")
	)

	err := flags.Parse(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	err = flagutil.SetFlagsFromEnv(flags, "TWITTER")
	if err != nil {
		log.Fatal(err)
	}

	if *consumerKey == "" ||
		*consumerSecret == "" ||
		*accessToken == "" ||
		*accessSecret == "" {
		log.Fatal(
			"Consumer key/secret and Access token/secret required. Got",
			" consumerKey: ", *consumerKey != "",
			" consumerSecret: ", *consumerSecret != "",
			" accessToken: ", *accessToken != "",
			" accessSecret: ", *accessSecret != "",
		)
	}

	var (
		config = oauth1.NewConfig(*consumerKey, *consumerSecret)
		token  = oauth1.NewToken(*accessToken, *accessSecret)

		// oAuth1 http.Client will automatically authorize Requests
		httpClient = config.Client(oauth1.NoContext, token)

		// Twitter client
		client = twitter.NewClient(httpClient)
	)

	// verify credentials
	verifyParams := &twitter.AccountVerifyParams{
		SkipStatus:   twitter.Bool(true),
		IncludeEmail: twitter.Bool(true),
	}
	_, _, errVerify := client.Accounts.VerifyCredentials(verifyParams)
	if errVerify != nil {
		log.Fatal("failed to verify credentials", errVerify)
	}

	fmt.Println("downloading likes")
	downloadLikedTweets(client, pathLikes)

	fmt.Println("downloading followed users")
	downloadFollowedUsers(client, pathFollowing)
}

func downloadLikedTweets(client *twitter.Client, path string) {

	var (
		start = time.Now()
		// Requests / 15-min window (app auth) = 75
		delay = (15*60)/75*time.Second +

			// add some extra to ensure we stay below the limit
			500*time.Millisecond

		yes         = true // who came up with the idea of using a bool pointer in the twitter api?? x)
		lastID      int64
		total       int
		numAssets   int
		first, last = time.Now(), time.Time{}
	)

	_ = os.RemoveAll(path)
	_ = os.Mkdir(path, pathPermission)

	for {
		favListParams := &twitter.FavoriteListParams{
			Count:           200,
			TweetMode:       "extended",
			IncludeEntities: &yes,
			MaxID:           lastID,
		}
		tweets, resp, err := client.Favorites.List(favListParams)
		if err != nil {
			log.Println(err)
			break
		}

		if resp.StatusCode == http.StatusOK {
			if len(tweets) > 0 {
				lastID = tweets[len(tweets)-1].ID
				for _, t := range tweets {

					ti := time.Unix(0, t.ID)
					if ti.Before(first) {
						first = ti
					}
					if ti.After(last) {
						last = ti
					}

					// save tweet
					filename := filepath.Join(path, t.IDStr+".json")
					_, err = os.Stat(filename)
					if err == nil {
						// file exists already
						continue
					}

					data, err := json.MarshalIndent(t, " ", "  ")
					if err != nil {
						log.Fatal(err)
					}
					err = os.WriteFile(filename, data, filePermission)
					if err != nil {
						log.Fatal("failed to write file: ", filename, " error: ", err)
					}

					// save media
					if len(t.Entities.Media) > 0 {

						mediaDir := filepath.Join(path, t.IDStr+"-media")
						err = os.Mkdir(mediaDir, pathPermission)
						if err != nil {
							log.Fatal("failed to create media directory: ", err)
						}

						for _, m := range t.ExtendedEntities.Media {

							//fmt.Println(" +", m.ExpandedURL, filepath.Base(m.MediaURL))

							resp, err := http.Get(m.MediaURL)
							if err != nil {
								log.Fatal(err)
							}
							if resp.StatusCode == http.StatusOK {
								data, err := ioutil.ReadAll(resp.Body)
								if err != nil {
									log.Fatal(err)
								}
								_ = resp.Body.Close()
								err = os.WriteFile(filepath.Join(mediaDir, filepath.Base(m.MediaURL)), data, filePermission)
								if err != nil {
									log.Fatal(err)
								}
								numAssets++
							} else {
								fmt.Println(resp.Status, "skipping")
							}
						}
					}
				}

				fmt.Println("+ downloaded", len(tweets), "tweets, total", total)
				total += len(tweets)
			} else {
				fmt.Println("done")
				break
			}
		} else {
			fmt.Println("unexpected status code", resp.Status)
			break
		}

		fmt.Println("sleeping for", delay)
		time.Sleep(delay)
	}

	size, err := directorySizeInBytes(path)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(total, "tweets and", numAssets, "media assets downloaded in", time.Since(start), "size on disk:", humanize.Bytes(uint64(size)))
	fmt.Println("contains tweets liked between", first.Format("02/01/2006"), "and", last.Format("02/01/2006"))
}

func downloadFollowedUsers(client *twitter.Client, path string) {

	var (
		// Requests / 15-min window (app auth) = 15
		delay = (15*60)/15*time.Second +

			// add some extra to ensure we stay below the limit
			500*time.Millisecond

		start  = time.Now()
		total  int
		yes          = true
		cursor int64 = -1

		_ = os.RemoveAll(path)
		_ = os.Mkdir(path, pathPermission)
	)

	for {
		favListParams := &twitter.FriendListParams{
			Count:               200,
			IncludeUserEntities: &yes,
			Cursor:              cursor,
		}
		friends, resp, err := client.Friends.List(favListParams)
		if err != nil {
			log.Println(err)
			break
		}
		if resp.StatusCode == http.StatusOK {
			if len(friends.Users) > 0 {

				for _, u := range friends.Users {

					// save user
					filename := filepath.Join(path, "/"+u.IDStr+".json")
					_, err := os.Stat(filename)
					if err == nil {
						// file exists already
						continue
					}

					data, err := json.MarshalIndent(u, " ", "  ")
					if err != nil {
						log.Fatal(err)
					}
					err = os.WriteFile(filename, data, filePermission)
					if err != nil {
						log.Fatal(err)
					}

					fmt.Println("+ saved user", u.Name, "https://twitter.com/"+u.ScreenName)
				}

				total += len(friends.Users)
			} else {
				fmt.Println("done")
				break
			}

			cursor = friends.NextCursor
			if cursor == 0 {
				break
			}
		} else {
			fmt.Println("unexpected status code", resp.Status)
			break
		}

		fmt.Println("sleeping for", delay, "total users downloaded", total)
		time.Sleep(delay)
	}

	size, err := directorySizeInBytes(path)
	if err != nil {
		log.Fatal("failed to determine directory size: ", err)
	}

	fmt.Println(total, "followed users downloaded in", time.Since(start), "size on disk:", humanize.Bytes(uint64(size)))
}

func directorySizeInBytes(path string) (size int64, err error) {
	err = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}
