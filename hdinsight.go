package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/urfave/cli/v2"
	"golang.org/x/sync/semaphore"
)

type HDInsight struct {
	ID            string            `json:"id"`
	ResourceGroup string            `json:"resourceGroup"`
	Name          string            `json:"name"`
	Location      string            `json:"location"`
	Properties    ClusterProperties `json:"properties"`
}
type ClusterProperties struct {
	ClusterDefinition ClusterDefinition `json:"clusterDefinition"`
	ComputeProfile    ComputeProfile    `json:"computeProfile"`
	CreatedDate       string            `json:"createdDate"`
}
type ClusterDefinition struct {
	Kind string `json:"kind"`
}
type ComputeProfile struct {
	Roles []Role `json:"roles"`
}
type Role struct {
	Name            string `json:"name"`
	HardwareProfile struct {
		VMSize string `json:"vmSize"`
	} `json:"hardwareProfile"`
	TargetInstanceCount int `json:"targetInstanceCount"`
}

func CheckHDInsight(c *cli.Context) error {
	client, err := NewClient(c.String("subscriptionID"))
	if err != nil {
		return err
	}
	fmt.Println("-------------------  getUnusedHDInsight -----------------------")
	h, err2 := getUnusedCluster(client, client.SubscriptionID)
	if err2 != nil {
		return err2
	}
	fmt.Println("---------------------------------------------------------------")

	outputToFile(map[string][]HDInsight{"UnusedHDInsight": *h}, "result_hdinsight.html", "hdinsights.tmpl.html")
	outputToFile(*h, "result_hdinsight.csv", "hdinsights.tmpl.csv")
	return nil
}

func getCluster(client *Client, subscriptionID string) (*[]HDInsight, error) {
	project := []ResourceGraphQueryProject{
		{columnName: "id", queryProperty: "id"},
		{columnName: "resourceGroup", queryProperty: "resourceGroup"},
		{columnName: "name", queryProperty: "name"},
		{columnName: "location", queryProperty: "location"},
		{columnName: "properties", queryProperty: "properties"},
	}

	qr := buildQueryRequest(
		`resources | where type =~ "microsoft.hdinsight/clusters"`,
		subscriptionID,
		project,
	)

	r, err := FetchResourceGraphData(context.TODO(), client, qr, &HDInsight{})
	if err != nil {
		fmt.Println(qr.query)
		return nil, err
	}
	var result []HDInsight
	for _, d := range r {
		h := *d.(*HDInsight)
		result = append(result, h)
	}

	return &result, nil
}
func getUnusedCluster(client *Client, subscriptionID string) (*[]HDInsight, error) {

	// --------------------------------------------
	// HDInsight の一覧を取得
	// --------------------------------------------
	clusters, err := getCluster(client, subscriptionID)
	if err != nil {
		return nil, err
	}
	// --------------------------------------------
	// 取得した HDInsight のメトリックを取得
	// --------------------------------------------
	var unusedHDInsight []HDInsight

	var wg sync.WaitGroup
	mutex := &sync.Mutex{}

	s := semaphore.NewWeighted(QueryConcurrency)

	for _, elem := range *clusters {
		elem := elem
		wg.Add(1)
		s.Acquire(context.Background(), 1)

		go func() error {
			defer s.Release(1)
			defer wg.Done()
			// 過去1か月1つも Gateway Requests がないクラスタ
			input := FetchMetricDataInput{
				subscriptionID:   subscriptionID,
				namespace:        "microsoft.hdinsight/clusters",
				resource:         elem.Name,
				resourceGroup:    elem.ResourceGroup,
				aggregation:      "Average",
				metricNames:      []string{"GatewayRequests"},
				timeDurationHour: 24 * 30,
			}
			fmt.Printf("Processing... get metric:%s\n", elem.Name)
			metricsList, err := FetchMetricData(context.TODO(), client, input)
			if err != nil {
				return err
			}

			// Gateway Requests があるクラスタは無視
			if len(metricsList["GatewayRequests"]) > 0 {
				return nil
			}
			// 1か月全体の平均を算出
			var avg float64
			for _, gr := range metricsList["GatewayRequests"] {
				avg += *gr.Average
			}
			avg /= float64(len(metricsList["GatewayRequests"]))

			mutex.Lock()
			unusedHDInsight = append(unusedHDInsight, elem)
			mutex.Unlock()
			return nil
		}()
	}
	wg.Wait()

	return &unusedHDInsight, nil
}
