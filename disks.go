package main

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/urfave/cli/v2"
	"golang.org/x/sync/semaphore"
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
	client, err := NewClient(c.String("subscriptionID"))
	if err != nil {
		return err
	}
	fmt.Println("-------------------  getUnattachedDisks -----------------------")
	disks, err2 := getUnattachedDisks(client, client.SubscriptionID)
	if err2 != nil {
		return err2
	}
	//for _, d := range *disks {
	//fmt.Printf("%s,%s,%s,%s,%d,%s,%s\n", d.ResourceGroup, d.Name, d.Sku.Name, d.Location, d.Properties.DiskSizeGB, d.Properties.DiskState, d.Properties.TimeCreated)
	//}
	fmt.Println("---------------------------------------------------------------")

	fmt.Println("-------------------  getUnusedVMDisks -----------------------")
	disks2, err3 := getUnusedVMDisks(client, client.SubscriptionID)
	if err3 != nil {
		return err3
	}
	//for _, d := range *disks2 {
	//fmt.Printf("%s,%s,%d\n", d.ID, d.Name, d.Properties.DiskSizeGB)
	//}
	fmt.Println("---------------------------------------------------------------")

	m := map[string][]Disk{}
	m["UnattachedDisks"] = *disks
	m["UnusedVMDisks"] = *disks2
	outputToHTML(m, "result_disks.html", "disks.tmpl.html")
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
		client.SubscriptionID,
		project,
	)
	fmt.Printf("Query:%s\n", qr.query)
	dl, err := FetchResourceGraphData(context.TODO(), client, qr, &Disk{})
	if err != nil {
		return nil, err
	}
	var result []Disk
	for _, d := range dl {
		result = append(result, *d.(*Disk))
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
	mutex := &sync.Mutex{}
	unusedVMID := []string{}

	var wg sync.WaitGroup

	s := semaphore.NewWeighted(QUERY_CONCURRENCY)
	for _, elem := range *vms {
		wg.Add(1)
		s.Acquire(context.Background(), 1)

		elem := elem

		go func() error {
			defer s.Release(1)
			defer wg.Done()

			fmt.Printf("Processing... get metric:%s\n", elem.Name)
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
				return err
			}

			// 1つもメトリックがない VM を使ってない VM とする
			if len(metricsList["Percentage CPU"]) == 0 {
				mutex.Lock()
				unusedVMID = append(unusedVMID, elem.ID)
				mutex.Unlock()
			}
			// テスト用
			//unusedVMID = append(unusedVMID, elem.ID)
			return nil

		}()

	}

	wg.Wait()

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
	diskProject := []ResourceGraphQueryProject{
		{columnName: "id", queryProperty: "id"},
		{columnName: "resourceGroup", queryProperty: "resourceGroup"},
		{columnName: "name", queryProperty: "name"},
		{columnName: "sku", queryProperty: "sku"},
		{columnName: "properties", queryProperty: "properties"},
		{columnName: "location", queryProperty: "location"},
	}

	var result []Disk

	mutex2 := &sync.Mutex{}
	var wg2 sync.WaitGroup
	s2 := semaphore.NewWeighted(QUERY_CONCURRENCY)

	// 一度に大量のクエリをすると制限があるため1クエリ当たりのディスク数の最大値を決める
	const QUERYNUM = 10
	loopCnt := int(math.Ceil(float64(len(unusedManagedDisksID)) / float64(QUERYNUM)))
	for i := 0; i < loopCnt; i++ {
		wg2.Add(1)
		s2.Acquire(context.Background(), 1)

		_tmp := unusedManagedDisksID[i*QUERYNUM : i*QUERYNUM+QUERYNUM]
		targetID := []string{}
		for _, v := range _tmp {
			if v != "" {
				targetID = append(targetID, v)
			}
		}

		queryMD := strings.ReplaceAll(strings.Join(targetID, `,`), `,`, `","`)

		qrMD := buildQueryRequest(
			`resources | where id in~("`+queryMD+`")`,
			subscriptionID,
			diskProject,
		)

		fmt.Printf("Query disk processing: %d/%d\n", i+1, loopCnt)
		go func() error {
			defer s2.Release(1)
			defer wg2.Done()
			r2, errFetchMDGraphData := FetchResourceGraphData(context.TODO(), client, qrMD, &Disk{})
			if errFetchMDGraphData != nil {
				fmt.Println(qr.query)
				return cli.NewExitError(fmt.Sprintf("fetch resource graph data failed: %s", errFetchMDGraphData.Error()), UNKNOWN)
			}

			mutex2.Lock()
			for _, d := range r2 {
				result = append(result, *d.(*Disk))
			}
			mutex2.Unlock()
			return nil
		}()
	}
	wg2.Wait()
	return &result, nil
}
