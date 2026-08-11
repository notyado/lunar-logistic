package store

import (
	"database/sql"
	"errors"

	"lunar-logistics/internal/model"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) init() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM game_state").Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return s.seed()
	}
	return nil
}

func (s *Store) Reset() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, table := range []string{"deliveries", "events", "orders", "rovers", "game_state"} {
		if _, err = tx.Exec("DELETE FROM " + table); err != nil {
			tx.Rollback()
			return err
		}
	}
	if _, err = tx.Exec(`DELETE FROM sqlite_sequence WHERE name IN ('deliveries','events','orders','rovers')`); err != nil {
		tx.Rollback()
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.seed()
}

func (s *Store) GetGame() (model.Game, error) {
	var g model.Game
	err := s.db.QueryRow(`SELECT day,max_days,credits,score,rating,goal,launches_today,finished FROM game_state WHERE id=1`).Scan(
		&g.Day, &g.MaxDays, &g.Credits, &g.Score, &g.Rating, &g.Goal, &g.LaunchesToday, &g.Finished,
	)
	g.BaseX = model.BaseX
	g.BaseY = model.BaseY
	return g, err
}

func (s *Store) GetOrder(id int64) (model.Order, error) {
	var o model.Order
	err := s.db.QueryRow(`SELECT id,code,title,category,weight,reward,deadline_day,risk,zone,x,y,status,container_image,distance,speed_factor,description FROM orders WHERE id=?`, id).Scan(
		&o.ID, &o.Code, &o.Title, &o.Category, &o.Weight, &o.Reward, &o.DeadlineDay, &o.Risk, &o.Zone, &o.X, &o.Y, &o.Status, &o.ContainerImage, &o.Distance, &o.SpeedFactor, &o.Description,
	)
	return o, err
}

func (s *Store) GetRover(id int64) (model.Rover, error) {
	var v model.Rover
	err := s.db.QueryRow(`SELECT id,slug,name,class,battery,max_battery,capacity,status,x,y,image,efficiency,speed FROM rovers WHERE id=?`, id).Scan(
		&v.ID, &v.Slug, &v.Name, &v.Class, &v.Battery, &v.MaxBattery, &v.Capacity, &v.Status, &v.X, &v.Y, &v.Image, &v.Efficiency, &v.Speed,
	)
	return v, err
}

func (s *Store) CountEnroute() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM deliveries WHERE status='enroute'`).Scan(&n)
	return n, err
}

func (s *Store) Snapshot(now int64) (model.State, error) {
	state := model.State{
		ServerTime: now,
		Rovers:     []model.Rover{},
		Orders:     []model.Order{},
		Deliveries: []model.Delivery{},
		Events:     []model.Event{},
	}
	var err error
	if state.Game, err = s.GetGame(); err != nil {
		return state, err
	}

	rows, err := s.db.Query(`SELECT id,slug,name,class,battery,max_battery,capacity,status,x,y,image,efficiency,speed FROM rovers ORDER BY id`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var v model.Rover
		if err = rows.Scan(&v.ID, &v.Slug, &v.Name, &v.Class, &v.Battery, &v.MaxBattery, &v.Capacity, &v.Status, &v.X, &v.Y, &v.Image, &v.Efficiency, &v.Speed); err != nil {
			rows.Close()
			return state, err
		}
		state.Rovers = append(state.Rovers, v)
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT id,code,title,category,weight,reward,deadline_day,risk,zone,x,y,status,container_image,distance,speed_factor,description FROM orders ORDER BY deadline_day,id`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var o model.Order
		if err = rows.Scan(&o.ID, &o.Code, &o.Title, &o.Category, &o.Weight, &o.Reward, &o.DeadlineDay, &o.Risk, &o.Zone, &o.X, &o.Y, &o.Status, &o.ContainerImage, &o.Distance, &o.SpeedFactor, &o.Description); err != nil {
			rows.Close()
			return state, err
		}
		state.Orders = append(state.Orders, o)
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT d.id,d.order_id,d.rover_id,d.status,d.started_at,d.completes_at,d.battery_cost,d.reward,d.risk,d.distance,d.result,
		o.x,o.y,o.title,r.name FROM deliveries d JOIN orders o ON o.id=d.order_id JOIN rovers r ON r.id=d.rover_id ORDER BY d.id DESC LIMIT 12`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var d model.Delivery
		if err = rows.Scan(&d.ID, &d.OrderID, &d.RoverID, &d.Status, &d.StartedAt, &d.CompletesAt, &d.BatteryCost, &d.Reward, &d.Risk, &d.Distance, &d.Result, &d.TargetX, &d.TargetY, &d.OrderTitle, &d.RoverName); err != nil {
			rows.Close()
			return state, err
		}
		state.Deliveries = append(state.Deliveries, d)
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT id,kind,message,created_at FROM events ORDER BY id DESC LIMIT 8`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var e model.Event
		if err = rows.Scan(&e.ID, &e.Kind, &e.Message, &e.CreatedAt); err != nil {
			rows.Close()
			return state, err
		}
		state.Events = append(state.Events, e)
	}
	rows.Close()
	return state, nil
}

var ErrNotFound = errors.New("not found")
