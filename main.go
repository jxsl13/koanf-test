package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func main() {

	err := NewRootCommand().Execute()
	if err != nil {
		log.Fatalln(err)
	}
}

func NewRootCommand() *cobra.Command {
	cfg := RootContext{}

	cmd := &cobra.Command{
		Use:  filepath.Base(os.Args[0]),
		RunE: cfg.RunE,
	}
	cmd.PreRunE = cfg.PreRunE(cmd)

	return cmd
}

type RootContext struct {
	RootConfig RootConfig
}

func (c *RootContext) PreRunE(cmd *cobra.Command) func(cmd *cobra.Command, args []string) error {
	parse := RegisterFlags(&c.RootConfig, false, cmd)
	return func(cmd *cobra.Command, args []string) error {
		return parse()
	}
}

func (c *RootContext) RunE(cmd *cobra.Command, args []string) error {
	b, err := json.MarshalIndent(c.RootConfig, "", " ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
