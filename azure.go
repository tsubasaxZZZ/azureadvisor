package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/preview/monitor/mgmt/2018-09-01/insights"
	"github.com/Azure/azure-sdk-for-go/services/preview/monitor/mgmt/2018-09-01/insights/insightsapi"
	"github.com/Azure/azure-sdk-for-go/services/resourcegraph/mgmt/2019-04-01/resourcegraph"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
)

// FetchMetricDataInput is input parameters for FetchMetricData
type FetchMetricDataInput struct {
	subscriptionID   string
	resourceGroup    string
	namespace        string
	resource         string
	metricNames      []string
	aggregation      string
	timeDurationHour int
}

// FetchMetricDefinitionsInput is input parameters for FetchMetricDefinitions
type FetchMetricDefinitionsInput struct {
	subscriptionID  string
	resourceGroup   string
	namespace       string
	resource        string
	resourceURI     string
	metricnamespace string
}

type ResourceGraphQueryRequestInput struct {
	subscriptionID string
	query          string
	facets         []string
}

// Client is an API Client for Azure
type Client struct {
	SubscriptionID          string
	MetricsClient           insightsapi.MetricsClientAPI
	MetricDefinitionsClient insightsapi.MetricDefinitionsClientAPI
	ResourceGraphClient     resourcegraph.OperationsClient
}

// NewClient returns *Client with setting Authorizer
func NewClient(subscriptionID string) (*Client, error) {
	//a, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	a, err := auth.NewAuthorizerFromCLI()
	if err != nil {
		return &Client{}, err
	}

	metricsClient := insights.NewMetricsClient(subscriptionID)
	metricsClient.Authorizer = a

	metricDefinitionsClient := insights.NewMetricDefinitionsClient(subscriptionID)
	metricDefinitionsClient.Authorizer = a

	resourceGraphClient := resourcegraph.NewOperationsClient()
	resourceGraphClient.Authorizer = a

	return &Client{
		SubscriptionID:          subscriptionID,
		MetricsClient:           metricsClient,
		MetricDefinitionsClient: metricDefinitionsClient,
		ResourceGraphClient:     resourceGraphClient,
	}, nil
}

type metricDefinitionsListInput struct {
	subscriptionID  string
	resourceURI     string
	metricnamespace string
}

type metricsListInput struct {
	subscriptionID  string
	resourceURI     string
	timespan        string
	interval        *string
	metricnames     string
	aggregation     string
	top             *int32
	orderby         string
	filter          string
	resultType      insights.ResultType
	metricnamespace string
}

func (c *Client) metricDefinitionsList(ctx context.Context, params *metricDefinitionsListInput) (insights.MetricDefinitionCollection, error) {
	return c.MetricDefinitionsClient.List(
		ctx,
		params.resourceURI,
		params.metricnamespace,
	)
}

func (c *Client) metricsList(ctx context.Context, params *metricsListInput) (insights.Response, error) {
	return c.MetricsClient.List(
		ctx,
		params.resourceURI,
		params.timespan,
		params.interval,
		params.metricnames,
		params.aggregation,
		params.top,
		params.orderby,
		params.filter,
		params.resultType,
		params.metricnamespace,
	)
}

// FetchMetricDefinitions returns metric definitions
func FetchMetricDefinitions(ctx context.Context, c *Client, params FetchMetricDefinitionsInput) (*[]insights.MetricDefinition, error) {
	input := &metricDefinitionsListInput{
		subscriptionID: params.subscriptionID,
		resourceURI: fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/%s/%s",
			params.subscriptionID,
			params.resourceGroup,
			params.namespace,
			params.resource,
		),
		metricnamespace: params.metricnamespace,
	}
	res, err := c.metricDefinitionsList(ctx, input)
	if err != nil {
		return nil, err
	}
	return res.Value, nil
}

// FetchMetricData fetches metric data and returns latest value with metric name as hash key
func FetchMetricData(ctx context.Context, c *Client, params FetchMetricDataInput) (map[string][]insights.MetricValue, error) {
	endTime := time.Now().UTC()
	startTime := endTime.Add(time.Duration(-1*params.timeDurationHour) * time.Hour)
	timespan := fmt.Sprintf("%s/%s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))

	var metricNames []string
	const metricsCountLimitPerRequest int = 20
	for {
		if len(params.metricNames) <= metricsCountLimitPerRequest {
			metricNames = append(metricNames, strings.Join(params.metricNames, ","))
			break
		}

		metricNames = append(metricNames, strings.Join(params.metricNames[:metricsCountLimitPerRequest], ","))
		params.metricNames = params.metricNames[metricsCountLimitPerRequest:]
	}

	metricsList := make(map[string][]insights.MetricValue)
	for _, m := range metricNames {
		//metrics := make(map[string]*insights.MetricValue)
		input := &metricsListInput{
			subscriptionID: params.subscriptionID,
			resourceURI: fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/%s/%s",
				params.subscriptionID,
				params.resourceGroup,
				params.namespace,
				params.resource,
			),
			timespan:    timespan,
			interval:    to.StringPtr("PT24H"),
			aggregation: params.aggregation,
			metricnames: m,
			resultType:  insights.Data,
		}
		res, err := c.metricsList(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, v := range *res.Value {
			for _, elem := range *v.Timeseries {
				for _, d := range *elem.Data {
					rv := reflect.ValueOf(d)
					av := rv.FieldByName(params.aggregation)
					if av.IsNil() {
						continue
					}

					if d.TimeStamp == nil {
						continue
					}
					metricsList[*v.Name.Value] = append(metricsList[*v.Name.Value], d)
				}

			}
		}
	}
	return metricsList, nil
}

func FetchResourceGraphData(c context.Context, client *Client, params ResourceGraphQueryRequestInput, v interface{}) ([]interface{}, error) {
	var facetRequest []resourcegraph.FacetRequest
	for i := 0; i < len(params.facets); i++ {
		facetRequest = append(
			facetRequest,
			resourcegraph.FacetRequest{
				Expression: &params.facets[i],
			},
		)
	}
	request := &resourcegraph.QueryRequest{
		Subscriptions: &[]string{params.subscriptionID},
		Query:         &params.query,
		Options:       &resourcegraph.QueryRequestOptions{ResultFormat: resourcegraph.ResultFormatObjectArray},
		Facets:        &facetRequest,
	}
	queryResponse, err := client.ResourceGraphClient.Resources(c, *request)
	if err != nil {
		return nil, err
	}

	var result []interface{}
	for _, elem := range queryResponse.Data.([]interface{}) {
		b, err := json.Marshal(elem)
		if err != nil {
			return nil, err
		}
		r := reflect.New(reflect.TypeOf(v).Elem()).Interface()
		errUnmarshal := json.Unmarshal(b, &r)
		if errUnmarshal != nil {
			return nil, errUnmarshal
		}
		result = append(result, r)
	}
	return result, nil
}
