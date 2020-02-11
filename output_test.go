package main

import (
	"testing"
)

func TestDiskOutput(t *testing.T) {
	disks1 := []Disk{}
	disks1 = append(disks1, Disk{
		ID:       "ID",
		Location: "JapanEast",
		Name:     "Disk",
		Properties: struct {
			DiskSizeGB  int    "json:\"diskSizeGB\""
			TimeCreated string "json:\"timeCreated\""
			DiskState   string "json:\"diskState\""
		}{
			DiskSizeGB: 10,
		},
		ResourceGroup: "ResourceGroup",
		Sku: struct {
			Name string "json:\"name\""
		}{
			Name: "Premium",
		},
	})
	disks1 = append(disks1, Disk{
		ID:       "ID",
		Location: "JapanEast",
		Name:     "Disk",
		Properties: struct {
			DiskSizeGB  int    "json:\"diskSizeGB\""
			TimeCreated string "json:\"timeCreated\""
			DiskState   string "json:\"diskState\""
		}{
			DiskSizeGB: 20,
		},
		ResourceGroup: "ResourceGroup",
		Sku: struct {
			Name string "json:\"name\""
		}{
			Name: "Premium",
		},
	})

	disks2 := []Disk{}
	disks2 = append(disks2, Disk{
		ID:       "ID",
		Location: "JapanWast",
		Name:     "Disk",
		Properties: struct {
			DiskSizeGB  int    "json:\"diskSizeGB\""
			TimeCreated string "json:\"timeCreated\""
			DiskState   string "json:\"diskState\""
		}{
			DiskSizeGB: 20,
		},
		ResourceGroup: "ResourceGroup",
		Sku: struct {
			Name string "json:\"name\""
		}{
			Name: "Premium",
		},
	})
	m := map[string][]Disk{}
	m["Data1"] = disks1
	m["Data2"] = disks2

	outputToHTML(m, "result_disks.html", "disks.html")

}
