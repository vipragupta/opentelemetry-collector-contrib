package push

import (
	"context"
	"log"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/pdata"
	controllerTime "go.opentelemetry.io/otel/sdk/metric/controller/time"
)

// DefaultPushPeriod is the default time interval between pushes.
const DefaultPushPeriod = 2 * time.Second

// Controller organizes a periodic push of metric data.
type Controller struct {
	lock         sync.Mutex
	exporter     component.MetricsExporter
	metric 		 pdata.Metrics
	wg           sync.WaitGroup
	ch           chan struct{}
	period       time.Duration
	timeout      time.Duration
	clock        controllerTime.Clock
	ticker       controllerTime.Ticker
}

// New constructs a Controller, an implementation of MeterProvider, using the
// provided checkpointer, exporter, and options to configure an SDK with
// periodic collection.
func New(exporter component.MetricsExporter, metric pdata.Metrics, opts ...Option) *Controller {
	c := &Config{
		Period: DefaultPushPeriod,
	}
	for _, opt := range opts {
		opt.Apply(c)
	}
	if c.Timeout == 0 {
		c.Timeout = c.Period
	}

	return &Controller{
		metric: metric,
		exporter:     exporter,
		ch:           make(chan struct{}),
		period:       c.Period,
		timeout:      c.Timeout,
		clock:        controllerTime.RealClock{},
	}
}

// SetClock supports setting a mock clock for testing.  This must be
// called before Start().
func (c *Controller) SetClock(clock controllerTime.Clock) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.clock = clock
}

// Start begins a ticker that periodically collects and exports
// metrics with the configured interval.
func (c *Controller) Start() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.ticker != nil {
		return
	}

	c.ticker = c.clock.Ticker(c.period)
	c.wg.Add(1)
	go c.run(c.ch)
}

// Stop waits for the background goroutine to return and then collects
// and exports metrics one last time before returning.
func (c *Controller) Stop() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.ch == nil {
		return
	}

	close(c.ch)
	c.ch = nil
	c.wg.Wait()
	c.ticker.Stop()

	c.tick()
}

func (c *Controller) run(ch chan struct{}) {
	for {
		select {
		case <-ch:
			c.wg.Done()
			return
		case <-c.ticker.C():
			c.tick()
		}
	}
}

func (c *Controller) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	for i:= 0; i < 3; i++ {
		err := c.exporter.ConsumeMetrics(ctx, c.metric)
		if err != nil {
			log.Printf("error while consuming the metric, %s", err)
			continue
		} else {
			break
		}
	}

	md := c.metric.ResourceMetrics().At(0).InstrumentationLibraryMetrics().At(0).Metrics()
	metrices := c.metric.MetricCount()
	for i:= 0; i < metrices; i++ {
		m := md.At(i).IntSum()
		m.InitEmpty()
	}
	log.Println("Published metrics")
	log.Printf("%s, %s", md.At(0).Name(), md.At(1).Name() )
	log.Println("-----------------------------------------------------------------")
}
