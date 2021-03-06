package app

import (
	"database/sql"
	"embed"
	"fmt"
	"gophermart/internal/app/config"
	"gophermart/internal/app/logger"
	"gophermart/internal/app/service/syncer"
	"gophermart/internal/app/session"
	"gophermart/internal/app/storage"
	"gophermart/internal/app/storage/postgres"
	"gophermart/pkg/accrual"
)

type App struct {
	config       config.Config
	logger       logger.Logger
	accrual      *accrual.Service
	users        storage.UserRepository
	orders       storage.OrderRepository
	transactions storage.TransactionRepository
	session      session.Manager
	stopCh       chan struct{}
	syncer       *syncer.Service
	db           *sql.DB
}

func New(cfg config.Config, logger logger.Logger, e embed.FS) (*App, error) {
	as, err := accrual.NewService(cfg.Accrual.RemoteURL)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("postgres", cfg.Database.DSN)
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}

	if err := applyMigrations(e, db); err != nil {
		return nil, fmt.Errorf("db migrate: %w", err)
	}

	users, err := postgres.NewUserRepository(db)
	if err != nil {
		return nil, fmt.Errorf("user repository init: %w", err)
	}

	orders, err := postgres.NewOrderRepository(db)
	if err != nil {
		return nil, fmt.Errorf("order repository init: %w", err)
	}

	transactions, err := postgres.NewTransactionRepository(db)
	if err != nil {
		return nil, fmt.Errorf("transaction repository init: %w", err)
	}

	s, err := syncer.New(db, as)
	if err != nil {
		return nil, fmt.Errorf("accryalsync init: %w", err)
	}

	a := &App{
		config:       cfg,
		logger:       logger,
		stopCh:       make(chan struct{}),
		users:        users,
		orders:       orders,
		transactions: transactions,
		session:      session.NewMemory(cfg.SecretKey, users),
		accrual:      as,
		syncer:       s,
		db:           db,
	}

	go func() {
		<-a.stopCh
		a.logger.Info().Msg("Shutting down application")
		s.Stop()
	}()

	return a, nil
}

func (a *App) Stop() {
	close(a.stopCh)
}
