package store

import (
	"time"

	"lunar-logistics/internal/model"
)

const schema = `
CREATE TABLE IF NOT EXISTS game_state (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	day INTEGER NOT NULL,
	max_days INTEGER NOT NULL,
	credits INTEGER NOT NULL,
	score INTEGER NOT NULL,
	rating INTEGER NOT NULL,
	goal INTEGER NOT NULL,
	launches_today INTEGER NOT NULL DEFAULT 0,
	finished INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS rovers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	slug TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	class TEXT NOT NULL,
	battery REAL NOT NULL,
	max_battery REAL NOT NULL,
	capacity REAL NOT NULL,
	status TEXT NOT NULL,
	x REAL NOT NULL,
	y REAL NOT NULL,
	image TEXT NOT NULL,
	efficiency REAL NOT NULL,
	speed REAL NOT NULL
);
CREATE TABLE IF NOT EXISTS orders (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	code TEXT NOT NULL UNIQUE,
	title TEXT NOT NULL,
	category TEXT NOT NULL,
	weight REAL NOT NULL,
	reward INTEGER NOT NULL,
	deadline_day INTEGER NOT NULL,
	risk INTEGER NOT NULL,
	zone TEXT NOT NULL,
	x REAL NOT NULL,
	y REAL NOT NULL,
	status TEXT NOT NULL,
	container_image TEXT NOT NULL,
	distance REAL NOT NULL,
	speed_factor REAL NOT NULL,
	description TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS deliveries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	order_id INTEGER NOT NULL REFERENCES orders(id),
	rover_id INTEGER NOT NULL REFERENCES rovers(id),
	status TEXT NOT NULL,
	started_at INTEGER NOT NULL,
	completes_at INTEGER NOT NULL,
	battery_cost REAL NOT NULL,
	reward INTEGER NOT NULL,
	risk INTEGER NOT NULL,
	distance REAL NOT NULL,
	result TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	kind TEXT NOT NULL,
	message TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_deliveries_status ON deliveries(status);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at DESC);
`

type roverSeed struct {
	slug, name, class, image string
	battery, capacity, eff, speed float64
}

type orderSeed struct {
	code, title, category, zone, image, description string
	weight                                          float64
	reward, deadline, risk                          int
	x, y, distance, speedFactor                     float64
}

var roverSeeds = []roverSeed{
	{"atlas", "ATLAS-8", "ТЯЖЁЛЫЙ", "/assets/rover-atlas.webp", 86, 260, 0.88, 0.78},
	{"selene", "SELENE-3", "СРЕДНИЙ", "/assets/rover-selene.webp", 78, 140, 1.00, 0.96},
	{"kite", "KITE-1", "ЛЁГКИЙ", "/assets/rover-kite.webp", 94, 55, 0.76, 1.18},
}

var orderSeeds = []orderSeed{
	{"LIFE-04", "Пост Армстронг", "ЖИЗНЕОБЕСПЕЧЕНИЕ", "Море Спокойствия", "/assets/container-life.webp", "Кислородные кассеты и аварийные рационы для смены геологов.", 42, 760, 1, 8, 69, 36, 54, 1.08},
	{"TECH-19", "Релейный узел Кеплер", "ТЕХНИКА", "Кратерный пояс", "/assets/container-tech.webp", "Силовые контроллеры для восстановления дальнего ретранслятора.", 108, 1240, 2, 26, 22, 28, 88, 0.78},
	{"REG-71", "Карьер Шеклтон-7", "РЕСУРСЫ", "Южные возвышенности", "/assets/container-resources.webp", "Пустые криокапсулы и буровой модуль. Маршрут тяжёлый и медленный.", 225, 1960, 4, 44, 84, 76, 108, 0.64},
	{"MED-08", "Медпост Тихо", "МЕДИЦИНА", "Бассейн Тихо", "/assets/container-life.webp", "Медицинский комплект и вода для изолированного научного поста.", 68, 940, 3, 16, 34, 82, 72, 0.96},
	{"POLAR-99", "Полярный массив N-9", "РЕСУРСЫ", "Теневая граница", "/assets/container-resources.webp", "Монолитный реакторный блок. Ни один доступный ровер не рассчитан на 310 кг.", 310, 2800, 5, 67, 10, 14, 138, 0.52},
}

func (s *Store) seed() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`INSERT INTO game_state(id,day,max_days,credits,score,rating,goal,launches_today,finished)
		VALUES(1,1,5,850,0,88,5000,0,0)`); err != nil {
		return err
	}
	for _, r := range roverSeeds {
		if _, err = tx.Exec(`INSERT INTO rovers(slug,name,class,battery,max_battery,capacity,status,x,y,image,efficiency,speed)
			VALUES(?,?,?,?,? ,?,'idle',?,?,?, ?,?)`,
			r.slug, r.name, r.class, r.battery, 100, r.capacity, model.BaseX, model.BaseY, r.image, r.eff, r.speed); err != nil {
			return err
		}
	}
	for _, o := range orderSeeds {
		if _, err = tx.Exec(`INSERT INTO orders(code,title,category,weight,reward,deadline_day,risk,zone,x,y,status,container_image,distance,speed_factor,description)
			VALUES(?,?,?,?,?,?,?,?,?,?,'available',?,?,?,?)`,
			o.code, o.title, o.category, o.weight, o.reward, o.deadline, o.risk, o.zone, o.x, o.y, o.image, o.distance, o.speedFactor, o.description); err != nil {
			return err
		}
	}
	now := time.Now().UnixMilli()
	if _, err = tx.Exec(`INSERT INTO events(kind,message,created_at) VALUES
		('system','Диспетчерская смена активна. Горизонт планирования — 5 суток.',?),
		('info','Три ровера прошли предстартовую диагностику.',?)`, now-1500, now-800); err != nil {
		return err
	}
	return tx.Commit()
}
