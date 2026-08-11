package model

const (
	BaseX = 48.0
	BaseY = 56.0
)

type Game struct {
	Day           int     `json:"day"`
	MaxDays       int     `json:"maxDays"`
	Credits       int     `json:"credits"`
	Score         int     `json:"score"`
	Rating        int     `json:"rating"`
	Goal          int     `json:"goal"`
	LaunchesToday int     `json:"launchesToday"`
	Finished      bool    `json:"finished"`
	BaseX         float64 `json:"baseX"`
	BaseY         float64 `json:"baseY"`
}

type Rover struct {
	ID         int64   `json:"id"`
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	Class      string  `json:"class"`
	Battery    float64 `json:"battery"`
	MaxBattery float64 `json:"maxBattery"`
	Capacity   float64 `json:"capacity"`
	Status     string  `json:"status"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Image      string  `json:"image"`
	Efficiency float64 `json:"efficiency"`
	Speed      float64 `json:"speed"`
}

type Order struct {
	ID             int64   `json:"id"`
	Code           string  `json:"code"`
	Title          string  `json:"title"`
	Category       string  `json:"category"`
	Weight         float64 `json:"weight"`
	Reward         int     `json:"reward"`
	DeadlineDay    int     `json:"deadlineDay"`
	Risk           int     `json:"risk"`
	Zone           string  `json:"zone"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Status         string  `json:"status"`
	ContainerImage string  `json:"containerImage"`
	Distance       float64 `json:"distance"`
	SpeedFactor    float64 `json:"speedFactor"`
	Description    string  `json:"description"`
}

type Delivery struct {
	ID          int64   `json:"id"`
	OrderID     int64   `json:"orderId"`
	RoverID     int64   `json:"roverId"`
	Status      string  `json:"status"`
	StartedAt   int64   `json:"startedAt"`
	CompletesAt int64   `json:"completesAt"`
	BatteryCost float64 `json:"batteryCost"`
	Reward      int     `json:"reward"`
	Risk        int     `json:"risk"`
	Distance    float64 `json:"distance"`
	Result      string  `json:"result"`
	TargetX     float64 `json:"targetX"`
	TargetY     float64 `json:"targetY"`
	OrderTitle  string  `json:"orderTitle"`
	RoverName   string  `json:"roverName"`
}

type Event struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Message   string `json:"message"`
	CreatedAt int64  `json:"createdAt"`
}

type State struct {
	Game       Game       `json:"game"`
	Rovers     []Rover    `json:"rovers"`
	Orders     []Order    `json:"orders"`
	Deliveries []Delivery `json:"deliveries"`
	Events     []Event    `json:"events"`
	ServerTime int64      `json:"serverTime"`
}

type DispatchRequest struct {
	OrderID int64 `json:"orderId"`
	RoverID int64 `json:"roverId"`
}
