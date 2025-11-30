package app

import (
	"context"
	"math"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/go-faster/errors"
	"github.com/povilasv/prommod"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"golang.org/x/sync/errgroup"

	"github.com/Educentr/go-project-starter-runtime/pkg/app/metrics"
	"github.com/Educentr/go-project-starter-runtime/pkg/ds"
	"github.com/Educentr/go-project-starter-runtime/pkg/model/actor"
)

// Все Init флаги вынести в пакет ready и на основе их сделать готовность приложения
type App struct {
	// Имя сервиса
	serviceName string

	// Имя приложения
	name string

	// Информация о сервисе
	info *ds.AppInfo

	// Сервис с бизнес логикой
	service ds.IService

	// Драйверы для работы с данными
	drivers []ds.Runnable

	// Драйверы инициализированы
	driverInit atomic.Bool

	// Авторизатор
	authorizer ds.Authorizer

	// Транспортный слой обслуживания бизнес логики
	transports []ds.RunnableService

	// Обработчики
	workers []ds.RunnableService

	// Transport initialized
	transportInit atomic.Bool

	// Worker initialized
	workerInit atomic.Bool

	// Готовность всего приложения к обслуживанию клиентов
	ready atomic.Bool

	// Контекст для остановки
	ctxStop context.CancelFunc

	// Группа обработки ошибок транспортов
	errGr *errgroup.Group

	// Метрики
	metrics *prometheus.Registry
}

type EmptyUserSetFunc struct{}

func (u *EmptyUserSetFunc) SetFunc(_ context.Context, _ *App) error { return nil }

type UnimplementedAuthorizer struct{}

func (u *UnimplementedAuthorizer) Init(_ context.Context, _ []ds.Runnable, _ *prometheus.Registry) (ds.Authorizer, error) {
	return u, nil
}

func (u *UnimplementedAuthorizer) AuthRest(r *http.Request) (ds.Actor, error) {
	return &actor.Actor{ID: math.MaxInt64}, nil
}

func (u *UnimplementedAuthorizer) CheckCSRF(r *http.Request) (bool, error) {
	return true, nil
}

var (
	errDriverNotInit        = errors.New("drivers not initialized")
	errDriverAlreadyInit    = errors.New("driver already initialized")
	errTransportNotInit     = errors.New("transports not initialized")
	errWorkerNotInit        = errors.New("worker not initialized")
	errTransportsEmpty      = errors.New("error initialize. No transports and no workers")
	errTransportAlreadyInit = errors.New("transport already initialized")
	errWorkerAlreadyInit    = errors.New("worker already initialized")
	errServiceEmpty         = errors.New("service is empty")
)

func New(ctx context.Context, serviceName, name string, info *ds.AppInfo) (*App, error) {
	app := &App{
		info:        info,
		name:        name,
		serviceName: serviceName,
	}

	var err error

	// Инициализируем метрики
	if app.metrics, err = metrics.InitMetrics(ctx); err != nil {
		return nil, err
	}

	nameForMetric := strings.ReplaceAll(serviceName+name, "-", "_")

	app.metrics.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
		collectors.NewBuildInfoCollector(),
		metrics.BuildInfoCollector(nameForMetric, app.info),
		prommod.NewCollector("server"),
	)

	return app, err
}

func (a *App) InitService(ctx context.Context) error {
	if !a.driverInit.Load() {
		return errDriverNotInit
	}

	bucket := ds.ServerBucket{AppInfo: a.info, AppReady: &a.ready}

	err := a.service.InitService(ctx, a.drivers, bucket, a.metrics)
	if err != nil {
		return errors.Wrap(err, "can't create new service")
	}

	return nil
}

func (a *App) SetTransport(transport ...ds.RunnableService) error {
	if a.transportInit.Load() {
		return errTransportAlreadyInit
	}

	a.transports = append(a.transports, transport...)

	return nil
}

func (a *App) SetWorker(worker ...ds.RunnableService) error {
	if a.workerInit.Load() {
		return errWorkerAlreadyInit
	}

	a.workers = append(a.workers, worker...)

	return nil
}

func (a *App) SetDriver(driver ...ds.Runnable) error {
	if a.driverInit.Load() {
		return errDriverAlreadyInit
	}

	a.drivers = append(a.drivers, driver...)

	return nil
}

func (a *App) InitTransports(ctx context.Context) error {
	if a.service == nil {
		return errServiceEmpty
	}

	if !a.transportInit.CompareAndSwap(false, true) {
		return errTransportAlreadyInit
	}

	for _, transport := range a.transports {
		if err := transport.Init(ctx, a.serviceName, a.name, a.metrics, a.service); err != nil {
			return errors.Wrapf(err, "can't create new router: %s", transport.Name())
		}
	}

	return nil
}

func (a *App) InitWorkers(ctx context.Context) error {
	if a.service == nil {
		return errServiceEmpty
	}

	if !a.workerInit.CompareAndSwap(false, true) {
		return errWorkerAlreadyInit
	}

	for _, worker := range a.workers {
		if err := worker.Init(ctx, a.serviceName, a.name, a.metrics, a.service); err != nil {
			return errors.Wrapf(err, "can't init worker: %s", worker.Name())
		}

		if err := worker.Initialization(ctx); err != nil {
			return errors.Wrapf(err, "can't initialize worker: %s", worker.Name())
		}
	}

	return nil
}

func (a *App) SetService(s ds.IService) error {
	a.service = s

	return nil
}

func (a *App) Init(ctx context.Context) error {
	// Initializing driver
	err := a.InitDrivers(ctx)
	if err != nil {
		return errors.Wrap(err, "can't initialize drivers")
	}

	err = a.InitService(ctx)
	if err != nil {
		return errors.Wrap(err, "can't initialize service")
	}

	err = a.InitTransports(ctx)
	if err != nil {
		return errors.Wrap(err, "can't initialize transport")
	}

	err = a.InitWorkers(ctx)
	if err != nil {
		return errors.Wrap(err, "can't initialize worker")
	}

	return nil
}

func (a *App) InitDrivers(ctx context.Context) error {
	if !a.driverInit.CompareAndSwap(false, true) {
		return errDriverAlreadyInit
	}

	bucket := ds.ServerBucket{AppInfo: a.info, AppReady: &a.ready}

	for _, driver := range a.drivers {
		if err := driver.Init(ctx, a.serviceName, bucket, a.metrics); err != nil {
			return errors.Wrapf(err, "can't initialize driver: %s", driver.Name())
		}
	}

	return nil
}

func (a *App) Run(ctx context.Context) error {
	if !a.driverInit.Load() {
		return errDriverNotInit
	}

	if !a.transportInit.Load() {
		return errTransportNotInit
	}

	if !a.workerInit.Load() {
		return errWorkerNotInit
	}

	if len(a.transports) == 0 && len(a.workers) == 0 {
		return errTransportsEmpty
	}

	if a.service == nil {
		return errServiceEmpty
	}

	for _, driver := range a.drivers {
		driver.Run(ctx, a.errGr)
	}

	err := a.service.BeforeRunHook(ctx)
	if err != nil {
		return errors.Wrap(err, "can't run service, before run hook failed")
	}

	for _, transport := range a.transports {
		transport.Run(ctx, a.errGr)
	}

	for _, worker := range a.workers {
		worker.Run(ctx, a.errGr)
	}

	// помечаем, что приложение запустилось
	a.ready.Store(true)
	<-ctx.Done()

	return a.gracefulStop(ctx)
}

func (a *App) Stop() error {
	return nil
}

// GetMetrics returns the prometheus registry for activerecord initialization
func (a *App) GetMetrics() *prometheus.Registry {
	return a.metrics
}
