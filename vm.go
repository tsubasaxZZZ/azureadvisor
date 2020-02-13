package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/urfave/cli/v2"
	"golang.org/x/sync/semaphore"
)

type VM struct {
	ID            string       `json:"id"`
	ResourceGroup string       `json:"resourceGroup"`
	Name          string       `json:"name"`
	Location      string       `json:"location"`
	Properties    VMProperties `json:"properties"`
	Zones         []string     `json:"zones"`
}

type VMProperties struct {
	StorageProfile struct {
		DataDisks DataDisks `json:"dataDisks"`
		OSDisk    OSDisk    `json:"osDisk"`
	} `json:"storageProfile"`
	HardwareProfile struct {
		VMSize string `json:"vmSize"`
	} `json:"hardwareProfile"`
}
type DataDisks []struct {
	Name         string `json:"name"`
	CreateOption string `json:"createOption"`
	Lun          int    `json:"lun"`
	ManagedDisk  struct {
		ID string `json:"id"`
	} `json:"managedDisk"`
}
type OSDisk struct {
	Name        string `json:"name"`
	ManagedDisk struct {
		ID string `json:"id"`
	} `json:"managedDisk"`
}

type RunningVM struct {
	VM                    VM
	PercentageCPUPerMonth float64
}

func CheckVM(c *cli.Context) error {
	client, err := NewClient(c.String("subscriptionID"))
	if err != nil {
		return err
	}
	fmt.Println("-------------------  getRunningVM -----------------------")
	vms, err4 := getRunningVM(client, client.SubscriptionID)
	if err4 != nil {
		return err4
	}
	fmt.Println("---------------------------------------------------------------")

	m := map[string][]RunningVM{}
	m["RunningVM"] = *vms
	outputToHTML(m, "result_vms.html", "vms.tmpl.html")
	return nil
}

func getVM(client *Client, subscriptionID string) (*[]VM, error) {
	project := []ResourceGraphQueryProject{
		{columnName: "id", queryProperty: "id"},
		{columnName: "resourceGroup", queryProperty: "resourceGroup"},
		{columnName: "name", queryProperty: "name"},
		{columnName: "properties", queryProperty: "properties"},
	}

	qr := buildQueryRequest(
		`resources | where type =~ "microsoft.compute/virtualmachines"`,
		subscriptionID,
		project,
	)

	r, err := FetchResourceGraphData(context.TODO(), client, qr, &VM{})
	if err != nil {
		fmt.Println(qr.query)
		return nil, err
	}
	var result []VM
	for _, d := range r {
		vm := *d.(*VM)
		result = append(result, vm)
	}

	return &result, nil
}

func getRunningVM(client *Client, subscriptionID string) (*[]RunningVM, error) {

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
	var runningVMs []RunningVM

	var wg sync.WaitGroup
	mutex := &sync.Mutex{}

	s := semaphore.NewWeighted(QueryConcurrency)

	for _, elem := range *vms {
		elem := elem
		wg.Add(1)
		s.Acquire(context.Background(), 1)

		go func() error {
			defer s.Release(1)
			defer wg.Done()
			// 過去1か月1つでもメトリックがある VM を利用している VM とする
			input := FetchMetricDataInput{
				subscriptionID:   subscriptionID,
				namespace:        "microsoft.compute/virtualmachines",
				resource:         elem.Name,
				resourceGroup:    elem.ResourceGroup,
				aggregation:      "Average",
				metricNames:      []string{"Percentage CPU"},
				timeDurationHour: 24 * 30,
			}
			fmt.Printf("Processing... get metric:%s\n", elem.Name)
			metricsList, err := FetchMetricData(context.TODO(), client, input)
			if err != nil {
				return err
			}

			// CPU 使用率がない VM はスキップ
			if len(metricsList["Percentage CPU"]) == 0 {
				return nil
			}
			// 1か月全体の平均を算出
			var avg float64
			for _, cpu := range metricsList["Percentage CPU"] {
				avg += *cpu.Average
			}
			avg /= float64(len(metricsList["Percentage CPU"]))

			mutex.Lock()
			runningVMs = append(runningVMs, RunningVM{VM: elem, PercentageCPUPerMonth: avg})
			mutex.Unlock()
			return nil
		}()
	}
	wg.Wait()

	return &runningVMs, nil

}
