package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

const (
	// QueryConcurrency is number of query concurrency
	QueryConcurrency = 20
)

func main() {
	app := &cli.App{
		Name:  "advisor",
		Usage: "Azure Advisor",
	}
	app.Commands = []*cli.Command{
		{
			Name:   "disk",
			Usage:  "Advisor for Disk",
			Action: CheckDisk,
		},
		{
			Name:   "vm",
			Usage:  "Advisor for VM",
			Action: CheckVM,
		},
		{
			Name:   "hdinsight",
			Usage:  "Advisor for HDInsight",
			Action: CheckHDInsight,
		},
	}
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:     "subscriptionID",
			Required: true,
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

type ResourceGraphQueryProject struct {
	columnName    string
	queryProperty string
}

type ResourceGraphResponse map[string]interface{}

func buildQueryRequest(baseQuery string, subscriptionID string, project []ResourceGraphQueryProject) ResourceGraphQueryRequestInput {
	q := baseQuery

	// project の組み立て
	q += "|project "
	for _, v := range project {
		q += fmt.Sprintf("%s=%s,", v.columnName, v.queryProperty)
	}
	q = strings.TrimRight(q, ",")

	return ResourceGraphQueryRequestInput{
		subscriptionID: subscriptionID,
		query:          q,
	}
}
