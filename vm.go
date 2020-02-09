package main

import (
	"context"
	"fmt"
)

func getVM(client *Client, subscriptionID string) (ResourceGraphResponse, string, error) {
	project := []ResourceGraphQueryProject{
		{columnName: "id", queryProperty: "id"},
		{columnName: "resourceGroup", queryProperty: "resourceGroup"},
		{columnName: "name", queryProperty: "name"},
		{columnName: "type", queryProperty: "type"},
	}

	qr := buildQueryRequest(
		`resources | where type =~ "microsoft.compute/virtualmachines"`,
		subscriptionID,
		project,
	)
	r, err := FetchResourceGraphData(context.TODO(), client, qr)
	if err != nil {
		fmt.Println(qr.query)
		return nil, "", err
	}

	stdout, err2 := buildStringResourceGraphResult(r, project)
	if err2 != nil {
		return nil, "", err2
	}
	return r, stdout, nil
}
