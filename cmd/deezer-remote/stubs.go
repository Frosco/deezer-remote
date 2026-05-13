package main

import "github.com/spf13/cobra"

func serveCmd() *cobra.Command  { return &cobra.Command{Use: "serve",  RunE: notImplemented} }
func pairCmd() *cobra.Command   { return &cobra.Command{Use: "pair",   RunE: notImplemented} }
func doctorCmd() *cobra.Command { return &cobra.Command{Use: "doctor", RunE: notImplemented} }

func notImplemented(*cobra.Command, []string) error { return nil }
