package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/MateSousa/create-release-bot/initializers"
	"github.com/google/go-github/v33/github"
	"golang.org/x/oauth2"
	"gorm.io/gorm/utils"
)

const (
	prLabelPending string = "createrelease:pending"
	prLabelmerged  string = "createrelease:merged"
	releaseCommit  string = "Release is at:"
	releaseName    string = "Release "
	mergeCommand   string = "/merge"
	mergeComment   string = "Merged by Create Release Bot"
)

func PREvent(client *github.Client, env initializers.Env, event *github.PullRequestEvent) error {
	switch *event.Action {
	case "closed":
		if !HasPendingLabel(event.PullRequest, nil) {
			return nil
		}
		// Remove label "createrelease:pending" from PR
		err := RemovePendingLabel(client, env, event.PullRequest, nil)
		if err != nil {
			fmt.Printf("error removing label: %v", err)
			return err
		}

	}
	return nil
}

func IssueEvent(client *github.Client, env initializers.Env, event *github.IssueCommentEvent) error {
	switch *event.Action {
	case "created":
		// Check if PR has label "createrelease:pending"
		if !HasPendingLabel(nil, event) {
			return nil
		}
		// Check if comment is "/merge"
		if !CheckIfCommentIsForMerge(*event.Comment.Body) {
			return nil
		}

		// Create/Update Changelog
		changelogCommit, err := CreateOrUpdateChangelog(client, env, event)
		if err != nil {
			return err
		}

		// sleep for 40 seconds
		time.Sleep(40 * time.Second)

		// Aprove Merge
		err = ApproveMerge(client, env, event)
		if err != nil {
			return err
		}

		// Remove label "createrelease:pending" and add "createrelease:merged" to PR
		err = AddMergedLabel(client, env, event)
		if err != nil {
			return err
		}

		// Create a new latest release tag and increment the minor version
		newReleaseTag, err := CreateNewLatestReleaseTag(client, env, *changelogCommit.SHA)
		if err != nil {
			return err
		}
		// Create a new release
		newRelease, err := CreateNewRelease(client, env, newReleaseTag)
		if err != nil {
			return err
		}
		// Create a new comment in PR with commit message
		commit := "Release is at: " + newRelease.GetHTMLURL()
		err = CreateNewComment(client, env, event, commit)
		if err != nil {
			return err
		}

	}

	return nil
}

func main() {
	env, err := initializers.LoadEnv()
	if err != nil {
		fmt.Printf("error loading env: %v", err)
		os.Exit(1)
	}

	client, err := CreateGithubClient(env)
	if err != nil {
		fmt.Printf("error creating github client: %v", err)
		os.Exit(1)
	}

	prEvent, _, err := ParsePullRequestEvent(env.GithubEvent, false)
	if err != nil {
		fmt.Printf("error parsing event: %v", err)
		os.Exit(1)
	}

	err = PREvent(client, env, prEvent)
	if err != nil {
		fmt.Printf("error handling pr event: %v", err)
		os.Exit(1)
	}

	_, issueEvent, err := ParsePullRequestEvent(env.GithubEvent, true)
	if err != nil {
		fmt.Printf("error parsing event: %v", err)
		os.Exit(1)
	}

	err = IssueEvent(client, env, issueEvent)
	if err != nil {
		fmt.Printf("error handling issue event: %v", err)
		os.Exit(1)
	}

	os.Exit(0)
}

// Create a github client with a token
func CreateGithubClient(env initializers.Env) (*github.Client, error) {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: env.Token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return client, nil
}

// Create a label "createrelease:pending" and add to PR
func AddPendingLabel(client *github.Client, env initializers.Env, pr *github.PullRequest) error {
	_, _, err := client.Issues.AddLabelsToIssue(context.Background(), env.RepoOwner, env.RepoName, *pr.Number, []string{prLabelPending})
	if err != nil {
		return err
	}

	return nil
}

func RemovePendingLabel(client *github.Client, env initializers.Env, pr *github.PullRequest, issueEvent *github.IssueCommentEvent) error {
	var prNumber int

	if pr != nil {
		prNumber = *pr.Number
	} else {
		prNumber = *issueEvent.Issue.Number
	}

	_, err := client.Issues.RemoveLabelForIssue(context.Background(), env.RepoOwner, env.RepoName, prNumber, prLabelPending)
	if err != nil {
		fmt.Printf("error removing label: %v", err)
		return err
	}

	return nil
}

// Remove label "createrelease:pending" and add "createrelease:merged" to PR
func AddMergedLabel(client *github.Client, env initializers.Env, pr *github.IssueCommentEvent) error {
	err := RemovePendingLabel(client, env, nil, pr)
	if err != nil {
		fmt.Printf("error removing label: %v", err)
		return err
	}

	_, _, err = client.Issues.AddLabelsToIssue(context.Background(), env.RepoOwner, env.RepoName, *pr.Issue.Number, []string{prLabelmerged})
	if err != nil {
		return err
	}

	return nil
}

// Create a new latest release tag and increment the minor version
func CreateNewLatestReleaseTag(client *github.Client, env initializers.Env, lastCommitSHA string) (string, error) {
	var releaseTag string

	releaseList, _, err := client.Repositories.ListReleases(context.Background(), env.RepoOwner, env.RepoName, nil)
	if err != nil {
		return "", err
	}

	noReleaseTag := len(releaseList) == 0
	if noReleaseTag {
		releaseTag = "v0.0.1"
	} else {
		latestReleaseTag := releaseList[0].GetTagName()
		latestReleaseTagSplit := strings.Split(latestReleaseTag, ".")

		latestReleaseTagMajorVersion, err := strconv.Atoi(latestReleaseTagSplit[0][1:])
		if err != nil {
			return "", err
		}

		latestReleaseTagMinorVersion, err := strconv.Atoi(latestReleaseTagSplit[1])
		if err != nil {
			return "", err
		}
		latestReleaseTagPatchVersion, err := strconv.Atoi(latestReleaseTagSplit[2])
		if err != nil {
			return "", err
		}

		if latestReleaseTagPatchVersion == 9 {
			latestReleaseTagMinorVersion = latestReleaseTagMinorVersion + 1
			latestReleaseTagPatchVersion = 0
		} else {
			latestReleaseTagPatchVersion = latestReleaseTagPatchVersion + 1
		}
		if latestReleaseTagMinorVersion == 9 && latestReleaseTagPatchVersion == 9 {
			latestReleaseTagMajorVersion = latestReleaseTagMajorVersion + 1
			latestReleaseTagMinorVersion = 0
			latestReleaseTagPatchVersion = 0
		}

		releaseTag = fmt.Sprintf("v%d.%d.%d", latestReleaseTagMajorVersion, latestReleaseTagMinorVersion, latestReleaseTagPatchVersion)
	}

	// Create a new tag
	now := time.Now()
	newReleaseTag, _, err := client.Git.CreateTag(context.Background(), env.RepoOwner, env.RepoName, &github.Tag{
		Tag:     &releaseTag,
		Message: &releaseTag,
		Object: &github.GitObject{
			Type: github.String("commit"),
			SHA:  github.String(lastCommitSHA),
		},
		Tagger: &github.CommitAuthor{
			Name:  github.String("Create Release Bot"),
			Email: github.String("githubaction@github.com"),
			Date:  &now,
		},
	})
	if err != nil {
		fmt.Printf("error creating tag: %v", err)
		return "", err
	}

	return *newReleaseTag.Tag, nil
}

// Create a new release
func CreateNewRelease(client *github.Client, env initializers.Env, newReleaseTag string) (*github.RepositoryRelease, error) {
	name := fmt.Sprintf("%s %s", releaseName, newReleaseTag)

	newRelease, _, err := client.Repositories.CreateRelease(context.Background(), env.RepoOwner, env.RepoName, &github.RepositoryRelease{
		TagName:         &newReleaseTag,
		Name:            &name,
		TargetCommitish: github.String("master"),
	})
	if err != nil {
		return nil, fmt.Errorf("error creating release mate: %v", err)
	}

	return newRelease, nil
}

// Create a new comment in PR with commit message
func CreateNewComment(client *github.Client, env initializers.Env, pr *github.IssueCommentEvent, commitMessage string) error {
	_, _, err := client.Issues.CreateComment(context.Background(), env.RepoOwner, env.RepoName, *pr.Issue.Number, &github.IssueComment{
		Body: &commitMessage,
	})
	if err != nil {
		return err
	}

	return nil
}

// Check if PR has label "createrelease:pending"
func HasPendingLabel(pr *github.PullRequest, issue *github.IssueCommentEvent) bool {
	// if pr is nil then it's a comment event
	if pr == nil {
		for _, label := range issue.Issue.Labels {
			if label.GetName() == prLabelPending {
				return true
			}
		}
	} else {
		for _, label := range pr.Labels {
			if label.GetName() == prLabelPending {
				return true
			}
		}
	}

	return false
}

func ParsePullRequestEvent(pullRequestEvent string, isIssueCommentEvent bool) (*github.PullRequestEvent, *github.IssueCommentEvent, error) {
	// Read the event payload from the env vars
	payloadEnv := pullRequestEvent
	if payloadEnv == "" {
		return nil, nil, fmt.Errorf("no payload found for event %s", pullRequestEvent)
	}

	// if it's a comment event, parse it as a comment event
	if isIssueCommentEvent {
		var issueCommentEvent github.IssueCommentEvent
		err := json.Unmarshal([]byte(payloadEnv), &issueCommentEvent)
		if err != nil {
			return nil, nil, err
		}

		return nil, &issueCommentEvent, nil
	} else {
		var pullRequestEvent github.PullRequestEvent
		err := json.Unmarshal([]byte(payloadEnv), &pullRequestEvent)
		if err != nil {
			return nil, nil, err
		}

		return &pullRequestEvent, nil, nil
	}
}

// Aprove merge create/update changelog and merge PR
func ApproveMerge(client *github.Client, env initializers.Env, pr *github.IssueCommentEvent) error {
	// Merge PR
	_, _, err := client.PullRequests.Merge(context.Background(), env.RepoOwner, env.RepoName, *pr.Issue.Number, mergeComment, nil)
	if err != nil {
		return err
	}

	return nil
}

// Get commits from PR
func GetPRCommits(client *github.Client, env initializers.Env, pr *github.IssueCommentEvent) []*github.RepositoryCommit {
	commits, _, err := client.PullRequests.ListCommits(context.Background(), env.RepoOwner, env.RepoName, *pr.Issue.Number, nil)
	if err != nil {
		return nil
	}

	return commits
}

// Get all commits from PR and create/update a CHANGELOG.md file with the changes from PR
func CreateOrUpdateChangelog(client *github.Client, env initializers.Env, pr *github.IssueCommentEvent) (*github.RepositoryContentResponse, error) {
	commits := GetPRCommits(client, env, pr)

	categorizedCommits, err := CategorizeCommits(commits)
	if err != nil {
		return nil, err
	}

	// Check if CHANGELOG.md file exists
	changelogFile, _, _, err := client.Repositories.GetContents(context.Background(), env.RepoOwner, env.RepoName, "CHANGELOG.md", nil)
	if err != nil {
		// If CHANGELOG.md file does not exist, create it
		if strings.Contains(err.Error(), "404") {
			commitChangelog, err := CreateChangelog(client, env, categorizedCommits, env.BaseBranch, *pr.Issue.Title)
			if err != nil {
				return nil, err
			}

			return commitChangelog, nil
		} else {
			return nil, err
		}
	} else {
		// If CHANGELOG.md file exists, update it
		commitChangelog, err := UpdateChangelog(client, env, categorizedCommits, env.BaseBranch, *changelogFile.SHA, *pr.Issue.Title)
		if err != nil {
			return nil, err
		}

		return commitChangelog, nil
	}

	return nil, nil

}

// Create CHANGELOG.md file
func CreateChangelog(client *github.Client, env initializers.Env, categorizedCommits map[string][]*github.RepositoryCommit, baseBranch string, releaseTag string) (*github.RepositoryContentResponse, error) {
	now := time.Now()

	var changelogContent string

	changelogContent = "# Changelog " + releaseTag + " (" + now.Format("2006-01-02") + ")\n\n"

	for commitType, commits := range categorizedCommits {
		changelogContent = changelogContent + "\n\n" + "## " + commitType + "\n\n"

		for _, commit := range commits {
			changelogContent = changelogContent + "- " + *commit.Commit.Message + "\n"
		}
	}

	commitMessage := "chore: create CHANGELOG.md file"
	commitContent := &github.RepositoryContentFileOptions{
		Message: github.String(commitMessage),
		Content: []byte(changelogContent),
		Branch:  github.String(baseBranch),
	}

	commitChangelog, _, err := client.Repositories.CreateFile(context.Background(), env.RepoOwner, env.RepoName, "CHANGELOG.md", commitContent)
	if err != nil {
		return nil, err
	}


	return commitChangelog, nil
}

// Update CHANGELOG.md file
func UpdateChangelog(client *github.Client, env initializers.Env, categorizedCommits map[string][]*github.RepositoryCommit, baseBranch string, lastCommitSHA string, releaseTag string) (*github.RepositoryContentResponse, error) {
	now := time.Now()

	var changelogContent string

	changelogContent = "# Changelog " + releaseTag + " (" + now.Format("2006-01-02") + ")\n\n"

	for commitType, commits := range categorizedCommits {
		changelogContent = changelogContent + "\n\n" + "## " + commitType + "\n\n"

		for _, commit := range commits {
			changelogContent = changelogContent + "- " + *commit.Commit.Message + "\n"
		}
	}

	commitMessage := "chore: update CHANGELOG.md file"
	commitContent := &github.RepositoryContentFileOptions{
		Message: github.String(commitMessage),
		Content: []byte(changelogContent),
		Branch:  github.String(baseBranch),
		SHA:     github.String(lastCommitSHA),
	}

	commitChangelog, _, err := client.Repositories.UpdateFile(context.Background(), env.RepoOwner, env.RepoName, "CHANGELOG.md", commitContent)
	if err != nil {
		return nil, err
	}

	return commitChangelog, nil

}

// Category each commits from PR using conventional commits
func CategorizeCommits(commits []*github.RepositoryCommit) (map[string][]*github.RepositoryCommit, error) {
	var conventionalCommitsType = []string{"feat", "fix", "perf", "refactor", "docs", "test", "chore", "style", "ci", "build"}

	categorizedCommits := make(map[string][]*github.RepositoryCommit)

	for _, commit := range commits {
		commitMessage := *commit.Commit.Message
		commitMessageSplit := strings.Split(commitMessage, ":")
		commitType := commitMessageSplit[0]

		// if commit type is contain any letter of conventional commits type then use it as commit type becasue some commits its like feat(main.go) or fix(main.go)
		for _, conventionalCommitsTypeLetter := range conventionalCommitsType {
			if strings.Contains(commitType, conventionalCommitsTypeLetter) {
				commitType = conventionalCommitsTypeLetter
				break
			}
		}

		// if commit type is not contain any letter of conventional commits type then use others as commit type
		if !utils.Contains(conventionalCommitsType, commitType) {
			commitType = "others"
		}

		categorizedCommits[commitType] = append(categorizedCommits[commitType], commit)

	}

	return categorizedCommits, nil
}

func CheckIfCommentIsForMerge(commentBody string) bool {
	if strings.Contains(commentBody, "/merge") {
		return true
	}

	return false
}
