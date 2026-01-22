//go:build ignore
// +build ignore

package server

// Shared cricket format codes used across ops status helpers.
// Keeping them centralized ensures consistent coverage/order.
var cricketFormatCodes = []string{"TEST", "ODI", "T20I", "T20"}

func getCricketFormats() []string { return cricketFormatCodes }
