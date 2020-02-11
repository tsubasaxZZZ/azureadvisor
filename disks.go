package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

const (
	OK       = 0
	WARNING  = 1
	CRITICAL = 2
	UNKNOWN  = 3
)

type Disk struct {
	ID            string `json:"id"`
	ResourceGroup string `json:"resourceGroup"`
	Name          string `json:"name"`
	Location      string `json:"location"`
	Sku           struct {
		Name string `json:"name"`
	} `json:"sku"`
	Properties struct {
		DiskSizeGB  int    `json:"diskSizeGB"`
		TimeCreated string `json:"timeCreated"`
		DiskState   string `json:"diskState"`
	} `json:"properties"`
}

func CheckDisk(c *cli.Context) error {
	s := "573d2a67-3e39-4f27-880e-5dd6bde361e1"
	client, err := NewClient(s)
	if err != nil {
		return err
	}
	disks, err2 := getUnattachedDisks(client, s)
	if err2 != nil {
		return err2
	}
	fmt.Println("-------------------  getUnattachedDisks -----------------------")
	for _, d := range *disks {
		fmt.Printf("%s,%s,%s,%s,%d,%s,%s\n", d.ResourceGroup, d.Name, d.Sku.Name, d.Location, d.Properties.DiskSizeGB, d.Properties.DiskState, d.Properties.TimeCreated)
	}
	fmt.Println("---------------------------------------------------------------")

	fmt.Println("-------------------  getUnusedVMDisks -----------------------")
	disks2, err3 := getUnusedVMDisks(client, s)
	if err3 != nil {
		return err3
	}
	for _, d := range *disks2 {
		fmt.Printf("%s,%s,%d\n", d.ID, d.Name, d.Properties.DiskSizeGB)
	}
	fmt.Println("---------------------------------------------------------------")

	return nil
}

func getUnattachedDisks(client *Client, subscriptionID string) (*[]Disk, error) {
	project := []ResourceGraphQueryProject{
		{columnName: "id", queryProperty: "id"},
		{columnName: "resourceGroup", queryProperty: "resourceGroup"},
		{columnName: "name", queryProperty: "name"},
		{columnName: "sku", queryProperty: "sku"},
		{columnName: "location", queryProperty: "location"},
		//{columnName: "diskSizeGB", queryProperty: "toint(properties.diskSizeGB)"},
		//{columnName: "diskState", queryProperty: "tostring(properties.diskState)"},
		//{columnName: "timeCreated", queryProperty: "properties.timeCreated"},
		{columnName: "properties", queryProperty: "properties"},
	}

	qr := buildQueryRequest(
		`resources | extend disk_tags =  bag_keys(tags) | extend disk_tags_string = tostring(disk_tags) | where type == "microsoft.compute/disks" | where properties.diskState == "Unattached" | where disk_tags_string !contains_cs "ASR-ReplicaDisk"`,
		subscriptionID,
		project,
	)
	dl, err := FetchResourceGraphData(context.TODO(), client, qr, &Disk{})
	if err != nil {
		return nil, err
	}
	var result []Disk
	for _, d := range dl {
		result = append(result, *d.(*Disk))
		//disk := d.(*Disk)
		//		fmt.Printf("%s,%s,%s,%s,%s,%d,%s,%s\n", disk.ID, disk.ResourceGroup, disk.Name, disk.SkuName, disk.Location, disk.DiskSizeGB, disk.DiskState, disk.TimeCreated)
	}

	return &result, nil
}

func getUnusedVMDisks(client *Client, subscriptionID string) (*[]Disk, error) {
	// --------------------------------------------
	// 仮想マシンの一覧を取得
	// --------------------------------------------
	vms, err := getVM(client, subscriptionID)
	if err != nil {
		return nil, err
	}
	// --------------------------------------------
	// 取得した仮想マシンのメトリックを取得
	// --------------------------------------------
	// TODO: 並列で取得
	unusedVMID := []string{}
	for _, elem := range *vms {
		input := FetchMetricDataInput{
			subscriptionID:   subscriptionID,
			namespace:        "microsoft.compute/virtualmachines",
			resource:         elem.Name,
			resourceGroup:    elem.ResourceGroup,
			aggregation:      "Average",
			metricNames:      []string{"Percentage CPU"},
			timeDurationHour: 24 * 30,
		}
		metricsList, err := FetchMetricData(context.TODO(), client, input)
		if err != nil {
			return nil, err
		}

		// 1つもメトリックがない VM を使ってない VM とする
		if len(metricsList["Percentage CPU"]) == 0 {
			unusedVMID = append(unusedVMID, elem.ID)
		}
		// テスト用
		//unusedVMID = append(unusedVMID, elem.ID)

	}

	// --------------------------------------------
	// 使用していない VM の 管理ディスクのID一覧を取得
	// --------------------------------------------
	project := []ResourceGraphQueryProject{
		{columnName: "id", queryProperty: "id"},
		{columnName: "osDisk", queryProperty: "properties.storageProfile.osDisk"},
		{columnName: "dataDisks", queryProperty: "properties.storageProfile.dataDisks"},
	}
	queryUnusedVMID := strings.ReplaceAll(strings.Join(unusedVMID, `,`), `,`, `","`)

	qr := buildQueryRequest(
		`resources | where id in~("`+queryUnusedVMID+`")`,
		subscriptionID,
		project,
	)

	type unusedVMIDs struct {
		ID        string    `json:"id"`
		OSDisk    OSDisk    `json:"osDisk"`
		DataDisks DataDisks `json:"dataDisks"`
	}
	r, errFetchGraphData := FetchResourceGraphData(context.TODO(), client, qr, &unusedVMIDs{})
	if errFetchGraphData != nil {
		fmt.Println(qr.query)
		return nil, cli.NewExitError(fmt.Sprintf("fetch resource graph data failed: %s", errFetchGraphData.Error()), UNKNOWN)
	}

	var unusedManagedDisksID []string

	for _, v := range r {
		vm := *v.(*unusedVMIDs)
		unusedManagedDisksID = append(unusedManagedDisksID, vm.OSDisk.ManagedDisk.ID)
		for _, d := range vm.DataDisks {
			unusedManagedDisksID = append(unusedManagedDisksID, d.ManagedDisk.ID)
		}
	}

	// ---------------------------------------------
	// 管理ディスクをクエリ
	// ---------------------------------------------
	// TODO: 並列で取得
	diskProject := []ResourceGraphQueryProject{
		{columnName: "id", queryProperty: "id"},
		{columnName: "resourceGroup", queryProperty: "resourceGroup"},
		{columnName: "name", queryProperty: "name"},
		{columnName: "sku", queryProperty: "sku"},
		{columnName: "properties", queryProperty: "properties"},
	}

	queryMD := strings.ReplaceAll(strings.Join(unusedManagedDisksID, `,`), `,`, `","`)

	qrMD := buildQueryRequest(
		`resources | where id in~("`+queryMD+`")`,
		subscriptionID,
		diskProject,
	)

	r2, errFetchMDGraphData := FetchResourceGraphData(context.TODO(), client, qrMD, &Disk{})
	if errFetchMDGraphData != nil {
		fmt.Println(qr.query)
		return nil, cli.NewExitError(fmt.Sprintf("fetch resource graph data failed: %s", errFetchMDGraphData.Error()), UNKNOWN)
	}

	var result []Disk
	for _, d := range r2 {
		result = append(result, *d.(*Disk))
	}

	return &result, nil
}
