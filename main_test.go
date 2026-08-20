package main

import (
	"bytes"
	"testing"
)

func TestRootCommandTreeExecutes(t *testing.T) {
	configureRootCommand()

	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	rootCmd.SetArgs([]string{"--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute root command: %v", err)
	}

	providerModel, _, err := rootCmd.Find([]string{"provider", "model"})
	if err != nil {
		t.Fatalf("find provider model command: %v", err)
	}
	if providerModel.CommandPath() != "omnillm provider model" {
		t.Fatalf("provider model command path = %q", providerModel.CommandPath())
	}

	legacyModel, _, err := rootCmd.Find([]string{"model"})
	if err != nil {
		t.Fatalf("find legacy model command: %v", err)
	}
	if !legacyModel.Hidden {
		t.Fatal("root model compatibility command must remain hidden")
	}

	output.Reset()
	rootCmd.SetArgs([]string{"model", "--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute legacy model help: %v", err)
	}
}
