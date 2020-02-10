package main

import (
	"context"
	"fmt"
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
}
type DataDisks []struct {
	Name        string `json:"name"`
	DiskSizeGB  int    `json:"diskSizeGB"`
	ManagedDisk struct {
		ID string `json:"id"`
	}
}
type OSDisk struct {
	Name        string `json:"name"`
	ManagedDisk struct {
		ID string `json:"id"`
	}
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
