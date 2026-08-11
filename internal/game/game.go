package game

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"lunar-logistics/internal/model"
	"lunar-logistics/internal/store"
)

type Engine struct {
	store *store.Store
	mu    sync.Mutex
}

func New(s *store.Store) *Engine {
	return &Engine{store: s}
}

func (e *Engine) State() (model.State, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now().UnixMilli()
	if err := e.reconcile(now); err != nil {
		return model.State{}, err
	}
	return e.store.Snapshot(now)
}

type DispatchInput struct {
	OrderID int64
	RoverID int64
}

func (e *Engine) Dispatch(in DispatchInput) (model.State, error) {
	if in.OrderID < 1 || in.RoverID < 1 {
		return model.State{}, errors.New("некорректный запрос диспетчеризации")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now().UnixMilli()
	if err := e.reconcile(now); err != nil {
		return model.State{}, err
	}

	game, err := e.store.GetGame()
	if err != nil {
		return model.State{}, err
	}
	if game.Finished {
		return model.State{}, errors.New("экспедиция уже завершена")
	}

	o, err := e.store.GetOrder(in.OrderID)
	if err != nil {
		return model.State{}, errors.New("заказ не найден")
	}
	rover, err := e.store.GetRover(in.RoverID)
	if err != nil {
		return model.State{}, errors.New("ровер не найден")
	}
	if o.Status != "available" {
		return model.State{}, errors.New("заказ уже недоступен")
	}
	if rover.Status != "idle" {
		return model.State{}, errors.New("ровер уже выполняет доставку")
	}
	if game.Day > o.DeadlineDay {
		return model.State{}, errors.New("окно доставки закрыто")
	}
	if rover.Capacity < o.Weight {
		return model.State{}, fmt.Errorf("грузоподъёмность ниже требуемых %.0f кг", o.Weight)
	}
	cost := BatteryCost(o, rover)
	if rover.Battery+0.001 < cost {
		return model.State{}, fmt.Errorf("нужно %.1f%% заряда, доступно %.1f%%", cost, rover.Battery)
	}

	duration := DurationSeconds(o, rover)
	completeAt := now + int64(duration*1000)
	tx, err := e.store.DB().Begin()
	if err != nil {
		return model.State{}, err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`INSERT INTO deliveries(order_id,rover_id,status,started_at,completes_at,battery_cost,reward,risk,distance,result)
		VALUES(?,?,'enroute',?,?,?,?,?,?,'')`, o.ID, rover.ID, now, completeAt, cost, o.Reward, o.Risk, o.Distance); err != nil {
		return model.State{}, err
	}
	if _, err = tx.Exec(`UPDATE orders SET status='delivering' WHERE id=?`, o.ID); err != nil {
		return model.State{}, err
	}
	if _, err = tx.Exec(`UPDATE rovers SET status='enroute', battery=MAX(0,battery-?) WHERE id=?`, cost, rover.ID); err != nil {
		return model.State{}, err
	}
	if _, err = tx.Exec(`UPDATE game_state SET launches_today=launches_today+1 WHERE id=1`); err != nil {
		return model.State{}, err
	}
	msg := fmt.Sprintf("%s → %s. Маршрут подтверждён, расход %.1f%%.", rover.Name, o.Title, cost)
	if _, err = tx.Exec(`INSERT INTO events(kind,message,created_at) VALUES('launch',?,?)`, msg, now); err != nil {
		return model.State{}, err
	}
	if err = tx.Commit(); err != nil {
		return model.State{}, err
	}
	return e.store.Snapshot(now)
}

func (e *Engine) NextDay() (model.State, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now().UnixMilli()
	if err := e.reconcile(now); err != nil {
		return model.State{}, err
	}
	active, err := e.store.CountEnroute()
	if err != nil {
		return model.State{}, err
	}
	if active > 0 {
		return model.State{}, errors.New("дождитесь возвращения активных роверов")
	}
	game, err := e.store.GetGame()
	if err != nil {
		return model.State{}, err
	}
	if game.Finished {
		return model.State{}, errors.New("экспедиция уже завершена")
	}

	tx, err := e.store.DB().Begin()
	if err != nil {
		return model.State{}, err
	}
	defer tx.Rollback()
	if game.Day >= game.MaxDays {
		if _, err = tx.Exec(`UPDATE game_state SET finished=1 WHERE id=1`); err == nil {
			_, err = tx.Exec(`INSERT INTO events(kind,message,created_at) VALUES('system','Экспедиционное окно закрыто. Итоговый отчёт сформирован.',?)`, now)
		}
	} else {
		newDay := game.Day + 1
		var expired int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM orders WHERE status='available' AND deadline_day < ?`, newDay).Scan(&expired); err == nil {
			_, err = tx.Exec(`UPDATE orders SET status='expired' WHERE status='available' AND deadline_day < ?`, newDay)
		}
		if err == nil {
			_, err = tx.Exec(`UPDATE game_state SET day=?, launches_today=0, rating=MAX(0,rating-?) WHERE id=1`, newDay, expired*3)
		}
		if err == nil {
			_, err = tx.Exec(`UPDATE rovers SET battery=MIN(max_battery,battery+28), status='idle', x=?, y=?`, model.BaseX, model.BaseY)
		}
		if err == nil {
			msg := fmt.Sprintf("Сутки %d начались. Сервисный цикл восстановил +28%% заряда.", newDay)
			if expired > 0 {
				msg += fmt.Sprintf(" Просрочено заказов: %d.", expired)
			}
			_, err = tx.Exec(`INSERT INTO events(kind,message,created_at) VALUES('day',?,?)`, msg, now)
		}
	}
	if err != nil {
		return model.State{}, err
	}
	if err = tx.Commit(); err != nil {
		return model.State{}, err
	}
	return e.store.Snapshot(now)
}

func (e *Engine) Reset() (model.State, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.store.Reset(); err != nil {
		return model.State{}, err
	}
	return e.store.Snapshot(time.Now().UnixMilli())
}

type dueDelivery struct {
	id, orderID, roverID int64
	reward, risk, day    int
	title, rover         string
}

func (e *Engine) reconcile(now int64) error {
	rows, err := e.store.DB().Query(`SELECT d.id,d.order_id,d.rover_id,d.reward,d.risk,g.day,o.title,r.name
		FROM deliveries d JOIN game_state g ON g.id=1 JOIN orders o ON o.id=d.order_id JOIN rovers r ON r.id=d.rover_id
		WHERE d.status='enroute' AND d.completes_at<=?`, now)
	if err != nil {
		return err
	}
	var dueList []dueDelivery
	for rows.Next() {
		var d dueDelivery
		if err := rows.Scan(&d.id, &d.orderID, &d.roverID, &d.reward, &d.risk, &d.day, &d.title, &d.rover); err != nil {
			rows.Close()
			return err
		}
		dueList = append(dueList, d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, d := range dueList {
		if err := e.resolveDelivery(d, now); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) resolveDelivery(d dueDelivery, now int64) error {
	tx, err := e.store.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	roll := int((d.id*37 + d.orderID*19 + int64(d.day)*23) % 100)
	success := roll >= d.risk
	if success {
		var deadline int
		if err = tx.QueryRow(`SELECT deadline_day FROM orders WHERE id=?`, d.orderID).Scan(&deadline); err != nil {
			return err
		}
		payout := d.reward
		if d.day < deadline {
			payout += int(math.Round(float64(d.reward) * 0.10))
		}
		if _, err = tx.Exec(`UPDATE deliveries SET status='completed',result='success',reward=? WHERE id=?`, payout, d.id); err == nil {
			_, err = tx.Exec(`UPDATE orders SET status='delivered' WHERE id=?`, d.orderID)
		}
		if err == nil {
			_, err = tx.Exec(`UPDATE rovers SET status='idle',x=?,y=? WHERE id=?`, model.BaseX, model.BaseY, d.roverID)
		}
		if err == nil {
			_, err = tx.Exec(`UPDATE game_state SET credits=credits+?,score=score+?,rating=MIN(100,rating+2) WHERE id=1`, payout, payout)
		}
		if err == nil {
			msg := fmt.Sprintf("%s: груз «%s» доставлен. Зачислено %d кр.", d.rover, d.title, payout)
			_, err = tx.Exec(`INSERT INTO events(kind,message,created_at) VALUES('success',?,?)`, msg, now)
		}
	} else {
		penalty := max(4, d.risk/8)
		if _, err = tx.Exec(`UPDATE deliveries SET status='completed',result='failed',reward=0 WHERE id=?`, d.id); err == nil {
			_, err = tx.Exec(`UPDATE orders SET status='failed' WHERE id=?`, d.orderID)
		}
		if err == nil {
			_, err = tx.Exec(`UPDATE rovers SET status='idle',x=?,y=? WHERE id=?`, model.BaseX, model.BaseY, d.roverID)
		}
		if err == nil {
			_, err = tx.Exec(`UPDATE game_state SET score=MAX(0,score-180),rating=MAX(0,rating-?) WHERE id=1`, penalty)
		}
		if err == nil {
			msg := fmt.Sprintf("%s: доставка «%s» сорвана рельефом. Рейтинг −%d.", d.rover, d.title, penalty)
			_, err = tx.Exec(`INSERT INTO events(kind,message,created_at) VALUES('failure',?,?)`, msg, now)
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func BatteryCost(o model.Order, r model.Rover) float64 {
	cost := (o.Distance*0.45+o.Weight*0.06)/r.Efficiency + float64(o.Risk)*0.08
	return math.Ceil(cost*10) / 10
}

func DurationSeconds(o model.Order, r model.Rover) int {
	routeFactor := math.Max(0.45, o.SpeedFactor)
	seconds := 5 + o.Distance/(r.Speed*routeFactor*20) + o.Weight/95 + float64(o.Risk)/35
	return max(7, int(math.Ceil(seconds)))
}
