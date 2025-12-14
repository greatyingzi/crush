package cmd

import (
	"errors"
	"os"
	"strings"

	"github.com/charmbracelet/crush/internal/ace"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(aceCmd)
	aceCmd.AddCommand(aceInitCmd, aceAddCmd, aceListCmd)

	aceInitCmd.Flags().BoolVarP(&aceInitForce, "force", "f", false, "Overwrite existing playbook")
	aceAddCmd.Flags().StringVar(&aceAddText, "text", "", "Key point text")
	aceAddCmd.Flags().StringSliceVar(&aceAddTags, "tags", nil, "Comma-separated tags (repeatable)")
	aceAddCmd.Flags().IntVar(&aceAddScore, "score", 0, "Initial score")
}

var aceCmd = &cobra.Command{
	Use:   "ace",
	Short: "Manage ACE project memory",
}

var aceInitForce bool

var aceInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize an empty ACE playbook",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfigForACE(cmd)
		if err != nil {
			return err
		}

		path := ace.PlaybookPath(cfg)
		if !aceInitForce {
			if _, err := os.Stat(path); err == nil {
				cmd.Printf("Playbook already exists: %s\n", path)
				return nil
			}
		}

		pb := ace.Playbook{Version: "1.0", KeyPoints: []ace.KeyPoint{}}
		if err := ace.NewFileStore().SaveAtomic(path, pb); err != nil {
			return err
		}
		cmd.Printf("Initialized playbook: %s\n", path)
		return nil
	},
}

var (
	aceAddText  string
	aceAddTags  []string
	aceAddScore int
)

var aceAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a key point to the ACE playbook",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfigForACE(cmd)
		if err != nil {
			return err
		}
		if strings.TrimSpace(aceAddText) == "" {
			return errors.New("--text is required")
		}

		path := ace.PlaybookPath(cfg)
		store := ace.NewFileStore()
		pb, err := store.Load(path)
		if err != nil {
			return err
		}

		kp := pb.Add(aceAddText, aceAddTags, aceAddScore)
		if err := store.SaveAtomic(path, pb); err != nil {
			return err
		}

		cmd.Printf("Added %s\n", kp.Name)
		return nil
	},
}

var aceListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List key points in the ACE playbook",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfigForACE(cmd)
		if err != nil {
			return err
		}

		path := ace.PlaybookPath(cfg)
		pb, err := ace.NewFileStore().Load(path)
		if err != nil {
			return err
		}

		if len(pb.KeyPoints) == 0 {
			cmd.Printf("No key points. Playbook: %s\n", path)
			return nil
		}

		for _, kp := range pb.KeyPoints {
			if kp.Pending {
				continue
			}
			if len(kp.Tags) > 0 {
				cmd.Printf("%s\t(score=%d)\t%s\t(tags: %s)\n", kp.Name, kp.Score, kp.Text, strings.Join(kp.Tags, ","))
				continue
			}
			cmd.Printf("%s\t(score=%d)\t%s\n", kp.Name, kp.Score, kp.Text)
		}
		return nil
	},
}

func loadConfigForACE(cmd *cobra.Command) (*config.Config, error) {
	debug, _ := cmd.Flags().GetBool("debug")
	dataDir, _ := cmd.Flags().GetString("data-dir")

	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Init(cwd, dataDir, debug)
	if err != nil {
		return nil, err
	}
	if err := createDotCrushDir(cfg.Options.DataDirectory); err != nil {
		return nil, err
	}
	if cfg.Options.ACE == nil {
		cfg.Options.ACE = &config.ACEOptions{}
	}
	if cfg.Options.ACE.PlaybookPath == "" {
		cfg.Options.ACE.PlaybookPath = "ace/playbook.json"
	}
	return cfg, nil
}
