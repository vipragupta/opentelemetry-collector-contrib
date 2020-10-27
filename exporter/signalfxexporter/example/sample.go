// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/signalfxexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/signalfxexporter/push"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/pdata"
)


func initMeter() (component.MetricsExporter, context.Context){

	cfg := &signalfxexporter.Config{
		IngestURL:   "http://metrics-dev-pp-observability.us-central1.gcp.dev.paypalinc.com:12816/v2/datapoint",
		APIURL:      "http://localhost",
		AccessToken: "JD5uxzqGPjyLiAeXL-Qqwg",
	}

	factory := signalfxexporter.NewFactory()
	params := component.ExporterCreateParams{}
	ctx := context.Background()
	exp, err := factory.CreateMetricsExporter(ctx, params, cfg)
	if err != nil {
		log.Fatalf("error while creating new exporter: %v", err)
	}

	return exp, ctx
}

func generateMetrics(datapoint pdata.IntSum, ctx context.Context, dimensions map[string]string,
	randomIndex int64, m *sync.Mutex, wg *sync.WaitGroup) {

	m.Lock()
	dp := pdata.NewIntDataPoint()
	dp.InitEmpty()
	dp.SetValue(rand.Int63n(randomIndex) + 1)
	dp.LabelsMap().InitFromMap(dimensions)
	dp.SetStartTime(pdata.TimestampUnixNano(uint64(time.Now().UnixNano())))
	dp.SetTimestamp(pdata.TimestampUnixNano(uint64(time.Now().UnixNano())))
	datapoint.DataPoints().Append(dp)
	log.Printf("value stored at %d: %d", datapoint.DataPoints().Len()-1, dp.Value())
	m.Unlock()
	wg.Done()
}

func initMetrics(md pdata.Metrics) (pdata.IntSum, pdata.IntSum){
	md.ResourceMetrics().Resize(1)
	md.ResourceMetrics().At(0).InstrumentationLibraryMetrics().Resize(1)

	// Get the list of all metric inside this Metrices at index 0
	metrics := md.ResourceMetrics().At(0).InstrumentationLibraryMetrics().At(0).Metrics()
	metrics.Resize(2)

	metric := metrics.At(0)
	metric.SetName("pp.caas.core.testrequest.count")
	metric.SetDescription("getting the total request")
	metric.SetUnit("1")
	metric.SetDataType(pdata.MetricDataTypeIntSum)
	sum := metric.IntSum()
	sum.InitEmpty()
	sum.SetAggregationTemporality(pdata.AggregationTemporalityUnspecified)

	metricTime := metrics.At(1)
	metricTime.SetName("pp.caas.core.request.total_duration.ms")
	metricTime.SetDescription("getting the total time took for the request")
	metricTime.SetUnit("1")
	metricTime.SetDataType(pdata.MetricDataTypeIntSum)
	timeTaken := metricTime.IntSum()
	timeTaken.InitEmpty()
	timeTaken.SetAggregationTemporality(pdata.AggregationTemporalityUnspecified)

	return sum, timeTaken
}

func shutdown(exp component.MetricsExporter, pusher *push.Controller, ctx context.Context) {
	exp.Shutdown(ctx)

	pusher.Stop()
}

func main() {
	exp, ctx := initMeter()
	exp.Start(ctx, componenttest.NewNopHost())

	md := pdata.NewMetrics()

	pusher := push.New( exp, md, push.WithPeriod(30*time.Second),)
	pusher.Start()

	defer shutdown(exp, pusher, ctx)

	sum, timeTaken :=  initMetrics(md)

	azValues:= []string{"dcg01", "dcg02", "dcg12", "dcg13", "dcg14", "dcg15", "ccg01", "ccg13", "ccg23"}
	requestTypesValues := []string{"create_pool", "delete_pool", "deploy", "patch_pool", "replace_instance", "enable_instance", "disable_instance"}
	subsystemValues := []string {"apirouter", "gfsm", "lfsm", "internal-api"}
	statusValues := []string {"success", "fail"}
	httpResponseValues:= []string{"200", "403", "500"}

	rand.Seed(time.Now().UnixNano())
	var m sync.Mutex
	var w sync.WaitGroup

	for i := 1; i < 10; i++ {
		dimensions := map[string]string{
			"az": azValues[rand.Int63n(int64(len(azValues)))],
			"request_type":  requestTypesValues[0],
			"sub_system": subsystemValues[0],
			"status": statusValues[0],
			"http_response_code": httpResponseValues[0],}

		w.Add(1)
		go generateMetrics(sum, ctx, dimensions, 100, &m, &w)
		w.Add(1)
		go generateMetrics(timeTaken, ctx, dimensions, 10, &m, &w)


		time.Sleep(1*time.Second)

		// if i %10 == 0 {
		// 	err := exp.ConsumeMetrics(ctx, md)
		// 	if err != nil {
		// 		log.Panicf("error while consuming the metric, %s", err)
		// 	}
		// 	sum.InitEmpty()
		// 	timeTaken.InitEmpty()
		// 	//sum.DataPoints().Resize(1)
		// 	//dataPoint := sum.DataPoints().At(0)
		// 	//dataPoint.SetStartTime(pdata.TimestampUnixNano(uint64(time.Now().UnixNano())))
		// 	log.Printf("Published %d", i)
		// 	log.Println("-----------------------------------------------------------------")
		// }
		// time.Sleep(1 * time.Second)
	}
	w.Wait()
}
