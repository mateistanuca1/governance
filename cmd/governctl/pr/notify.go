package pr

import (
	"context"
	"fmt"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/cobra"
	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/ghapi"
	"gopkg.in/yaml.v2"
	"kraftkit.sh/cmdfactory"
	kitcfg "kraftkit.sh/config"
)

type User struct {
	Github  string `yaml:"github"`
	Discord string `yaml:"discord"`
}

type ReviewNotifier struct {
	DiscordToken  string `long:"discord-token" env:"GOVERN_DISCORD_TOKEN" usage:"Discord API token"`
	DiscordClient *discordgo.Session
	ghClient      *ghapi.GithubClient
	users         map[string]User
}

func (opts *ReviewNotifier) LoadUsers(filePath string) error {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("could not read users.yaml: %w", err)
	}

	users := make(map[string]User)
	if err := yaml.Unmarshal(file, &users); err != nil {
		return fmt.Errorf("could not unmarshal users.yaml: %w", err)
	}

	opts.users = users
	return nil
}

func NewReviewNotifier() *cobra.Command {
	cmd, err := cmdfactory.New(&ReviewNotifier{}, cobra.Command{
		Use:   "notify-reviewers [OPTIONS] ORG/REPO/PRID",
		Short: "Notify reviewers via Discord for a pull request",
		Args:  cobra.MaximumNArgs(2),
	})
	if err != nil {
		panic(err)
	}
	return cmd
}

func (opts *ReviewNotifier) Run(ctx context.Context, args []string) error {
	var err error
	opts.DiscordClient, err = discordgo.New(opts.DiscordToken)
	opts.ghClient, err = ghapi.NewGithubClient(
		ctx,
		kitcfg.G[config.Config](ctx).GithubToken,
		kitcfg.G[config.Config](ctx).GithubSkipSSL,
		kitcfg.G[config.Config](ctx).GithubEndpoint,
	)
	if err != nil {
		return err
	}

	err = opts.LoadUsers("/home/matei/governance/cmd/governctl/pr/users.yaml")
	if err != nil {
		return err
	}

	ghOrg := "unikraft"
	ghRepo := "unikraft"
	prs, err := opts.ghClient.ListOpenPullRequests(
		ctx,
		ghOrg,
		ghRepo,
	)
	if err != nil {
		return fmt.Errorf("could not retrieve pull requests: %w", err)
	}

	// fmt.Printf("%d %#v", len(prs), prs)
	for _, pr := range prs {
		// fmt.Printf("%#v", *pr)
		reviewUsers, err := opts.ghClient.GetReviewUsersOnPr(ctx, ghOrg, ghRepo, int(*pr.Number))
		if err != nil {
			return fmt.Errorf("could not retrieve review users for PR #%d: %w", pr.Number, err)
		}
		for _, reviewUser := range reviewUsers {
			discordUser := ""
			for _, user := range opts.users {
				if user.Github == reviewUser {
					discordUser = user.Discord // Found the corresponding Discord username
					break
				}
			}
			fmt.Println(discordUser)
			if discordUser != "" {
				err := opts.NotifyReviewer(discordUser, reviewUser, *pr.Number)
				if err != nil {
					return fmt.Errorf("could not notify reviewer: %w", err)
				}
			} else {
				// If no Discord username is found, log it
				fmt.Printf("No Discord username found for GitHub user: %s\n", reviewUser)
			}

		}
	}
	return nil
}

func (opts *ReviewNotifier) NotifyReviewer(discordUser, githubUser string, prNumber int) error {
	user, err := opts.DiscordClient.GuildMembers()
	if err != nil {
		return fmt.Errorf("could not find Discord user %s: %w", discordUser, err)
	}

	st, err := opts.DiscordClient.UserChannelCreate(user.ID)

	message := fmt.Sprintf("Hello %s! You have a pull request #%d to review on GitHub (GitHub username: %s).", discordUser, prNumber, githubUser)
	_, err = opts.DiscordClient.ChannelMessageSend(st.ID, message)
	if err != nil {
		return fmt.Errorf("could not send message to Discord user %s: %w", discordUser, err)
	}

	fmt.Printf("Sent message to %s (%s): %s\n", discordUser, githubUser, message)
	return nil
}
