package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/urfave/cli/v2"
)

const (
	OK       = 0
	WARNING  = 1
	CRITICAL = 2
	UNKNOWN  = 3
)

func CheckDisk(c *cli.Context) error {
	s := "573d2a67-3e39-4f27-880e-5dd6bde361e1"
	client, err := NewClient(s)
	if err != nil {
		return err
	}
	_, _, err2 := getUnattachedDisks(client, s)
	if err2 != nil {
		return err2
	}
	//fmt.Print(stdout)
	_, stdout2, err3 := getUnusedVMDisks(client, s)
	if err3 != nil {
		return err3
	}
	fmt.Println(stdout2)
	return nil
}

func getUnattachedDisks(client *Client, subscriptionID string) (ResourceGraphResponse, string, error) {
	project := []ResourceGraphQueryProject{
		{columnName: "id", queryProperty: "id"},
		{columnName: "resourceGroup", queryProperty: "resourceGroup"},
		{columnName: "name", queryProperty: "name"},
		{columnName: "skuName", queryProperty: "sku.name"},
		{columnName: "location", queryProperty: "location"},
		{columnName: "diskSizeGB", queryProperty: "toint(properties.diskSizeGB)"},
		{columnName: "diskState", queryProperty: "tostring(properties.diskState)"},
		{columnName: "timeCreated", queryProperty: "properties.timeCreated"},
	}

	qr := buildQueryRequest(
		`resources | extend disk_tags =  bag_keys(tags) | extend disk_tags_string = tostring(disk_tags) | where type == "microsoft.compute/disks" | where properties.diskState == "Unattached" | where disk_tags_string !contains_cs "ASR-ReplicaDisk"`,
		subscriptionID,
		project,
	)
	r, err := FetchResourceGraphData(context.TODO(), client, qr)
	if err != nil {
		fmt.Println(qr.query)
		return nil, "", cli.NewExitError(fmt.Sprintf("fetch resource graph data failed: %s", err.Error()), UNKNOWN)
	}

	stdout, err2 := buildStringResourceGraphResult(r, project)
	if err2 != nil {
		return nil, "", cli.NewExitError(fmt.Sprintf("build string resource graph result failed: %s", err2.Error()), UNKNOWN)
	}
	return r, stdout, nil
}

func getUnusedVMDisks(client *Client, subscriptionID string) (ResourceGraphResponse, string, error) {
	// --------------------------------------------
	// 仮想マシンの一覧を取得
	// --------------------------------------------
	r, stdout, err := getVM(client, subscriptionID)
	if err != nil {
		return nil, "", err
	}
	// --------------------------------------------
	// 取得した仮想マシンのメトリックを取得
	// --------------------------------------------
	// TODO: 並列で取得
	unusedVMID := []string{}
	for _, elem := range r {
		/*
			input := FetchMetricDataInput{
				subscriptionID:   subscriptionID,
				namespace:        elem.(map[string]interface{})["type"].(string),
				resource:         elem.(map[string]interface{})["name"].(string),
				resourceGroup:    elem.(map[string]interface{})["resourceGroup"].(string),
				aggregation:      "Average",
				metricNames:      []string{"Percentage CPU"},
				timeDurationHour: 24 * 30,
			}
				metricsList, err := FetchMetricData(context.TODO(), client, input)
				if err != nil {
					return nil, "", err
				}

				// 1つもメトリックがない VM を使ってない VM とする
				if len(metricsList["Percentage CPU"]) == 0 {
					unusedVMID = append(unusedVMID, elem.(map[string]interface{})["id"].(string))
				}
		*/
		unusedVMID = append(unusedVMID, elem.(map[string]interface{})["id"].(string))

	}

	// --------------------------------------------
	// 使用していない VM の 管理ディスクのID一覧を取得
	// --------------------------------------------
	project := []ResourceGraphQueryProject{
		{columnName: "id", queryProperty: "id"},
		{columnName: "osDiskID", queryProperty: "properties.storageProfile.osDisk.managedDisk.id"},
		{columnName: "dataDisks", queryProperty: "properties.storageProfile.dataDisks"},
	}
	queryUnusedVMID := strings.ReplaceAll(strings.Join(unusedVMID, `,`), `,`, `","`)

	qr := buildQueryRequest(
		`resources | where id in~("`+queryUnusedVMID+`")`,
		subscriptionID,
		project,
	)
	qr.facets = []string{"dataDisks"}

	r, errFetchGraphData := FetchResourceGraphData(context.TODO(), client, qr)
	if errFetchGraphData != nil {
		fmt.Println(qr.query)
		return nil, "", cli.NewExitError(fmt.Sprintf("fetch resource graph data failed: %s", errFetchGraphData.Error()), UNKNOWN)
	}

	var unusedManagedDisksID []string

	for _, vm := range r {
		unusedManagedDisksID = append(unusedManagedDisksID, vm.(map[string]interface{})["osDiskID"].(string))
		for _, dataDisk := range vm.(map[string]interface{})["dataDisks"].([]interface{}) {
			dv := reflect.ValueOf(dataDisk)
			dm := dv.MapIndex(reflect.ValueOf("managedDisk"))
			if dm.IsValid() {
				unusedManagedDisksID = append(unusedManagedDisksID, dm.Interface().(map[string]interface{})["id"].(string))
			}
		}
	}

	// ---------------------------------------------
	// 管理ディスクをクエリ
	// ---------------------------------------------
	// TODO: 並列で取得
	type unusedManagedDiskInfo struct {
		resourceGroup string
		name          string
		sku           string
		diskSizeGB    int
	}
	//var unusedManagedDiskInfos []unusedManagedDiskInfo
	diskProject := []ResourceGraphQueryProject{
		{columnName: "id", queryProperty: "id"},
		{columnName: "resourceGroup", queryProperty: "resourceGroup"},
		{columnName: "name", queryProperty: "name"},
		{columnName: "sku", queryProperty: "sku.name"},
		{columnName: "diskSizeGB", queryProperty: "properties.diskSizeGB"},
	}

	queryMD := strings.ReplaceAll(strings.Join(unusedManagedDisksID, `,`), `,`, `","`)

	qrMD := buildQueryRequest(
		`resources | where id in~("`+queryMD+`")`,
		subscriptionID,
		diskProject,
	)
	r, errFetchMDGraphData := FetchResourceGraphData(context.TODO(), client, qrMD)
	if errFetchMDGraphData != nil {
		fmt.Println(qr.query)
		return nil, "", cli.NewExitError(fmt.Sprintf("fetch resource graph data failed: %s", errFetchMDGraphData.Error()), UNKNOWN)
	}

	stdout, _ = buildStringResourceGraphResult(r, diskProject)

	return r, stdout, nil
}
