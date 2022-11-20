# TWITTER-BACKUP

Simple tool for backing up your liked tweets and their media, as well as followed users as JSON to disk.

Get your access keys and tokens from: https://developer.twitter.com/en/portal/dashboard

and export in your .bashrc / .zshrc like this:

```
export TWITTER_CONSUMER_KEY="XXXXXXXXXXXXX"
export TWITTER_CONSUMER_SECRET="XXXXXXXXXXXXX"
export TWITTER_ACCESS_TOKEN="XXXXXXXXXXXXX"
export TWITTER_ACCESS_SECRET="XXXXXXXXXXXXX"
```

Afterwards, compile and run:

```
$ go build -o twitter-backup main.go
$ ./twitter-backup
```
