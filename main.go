package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"strings"

	"bytes"

	"encoding/json"

	"github.com/google/go-github/github"
)

var (
	reposFlag = flag.String("repos", "wallix/awless,wallix/triplestore,wallix/awless-templates,wallix/awless-scheduler", "CSV list of Github repos. Format: owner/repo,owner/repo")
)

type Week struct {
	T                             time.Time
	Additions, Deletions, Commits int
}

func (w *Week) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf(`{"week": "%s"`, w.T.Add(24*time.Hour).Format("Mon Jan 2 2006")))
	buf.WriteString(fmt.Sprintf(`, "commits": %d, "added": %d, "removed": %d}`, w.Commits, w.Additions, w.Deletions))

	return buf.Bytes(), nil
}

func main() {
	flag.Parse()

	client := github.NewClient(nil)

	repos := strings.Split(*reposFlag, ",")

	type result struct {
		err   error
		repo  string
		stats []*github.ContributorStats
	}
	resultc := make(chan *result)

	var wg sync.WaitGroup
	for _, r := range repos {
		wg.Add(1)
		go func(rep string) {
			defer wg.Done()
			owner, repo := splitOwnerRepo(rep)

			stats, _, err := client.Repositories.ListContributorsStats(context.Background(), owner, repo)
			resultc <- &result{stats: stats, err: err, repo: rep}
		}(r)
	}

	go func() {
		wg.Wait()
		close(resultc)
	}()

	byWeek := make(map[time.Time]*Week)
	for res := range resultc {
		if res.err != nil {
			log.Fatal(res.err)
		}
		for _, s := range res.stats {
			for _, w := range s.Weeks {
				commits := *w.Commits
				if commits > 0 {
					stamp := w.Week.Time
					if week, ok := byWeek[stamp]; !ok {
						byWeek[stamp] = &Week{T: stamp, Additions: *w.Additions, Deletions: *w.Deletions, Commits: commits}
					} else {
						week.Additions = week.Additions + *w.Additions
						week.Deletions = week.Deletions + *w.Deletions
						week.Commits = week.Commits + commits
					}
				}
			}
		}
	}

	var weeklyStats []*Week
	for _, w := range byWeek {
		weeklyStats = append(weeklyStats, w)
	}

	sort.Slice(weeklyStats, func(i, j int) bool { return weeklyStats[i].T.Before(weeklyStats[j].T) })

	b, err := json.MarshalIndent(weeklyStats, "", " ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))
}

func splitOwnerRepo(s string) (string, string) {
	splits := strings.Split(s, "/")
	switch len(splits) {
	case 2:
		return splits[0], splits[1]
	default:
		return "", ""
	}
}
