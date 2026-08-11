package server

import (
        "encoding/json"
        "errors"
        "io/fs"
        "log"
        "net/http"
        "os"
        "strconv"
        "strings"

        "lunar-logistics/internal/game"
        "lunar-logistics/internal/model"
)

type Server struct {
        engine *game.Engine
        web    fs.FS
}

func New(engine *game.Engine, web fs.FS) *Server {
        return &Server{engine: engine, web: web}
}

func (s *Server) Handler() http.Handler {
        mux := http.NewServeMux()
        mux.HandleFunc("GET /api/state", s.handleState)
        mux.HandleFunc("POST /api/dispatch", s.handleDispatch)
        mux.HandleFunc("POST /api/next-day", s.handleNextDay)
        mux.HandleFunc("POST /api/reset", s.handleReset)
        mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
                writeJSON(w, http.StatusOK, map[string]string{"status": "operational"})
        })
        mux.Handle("/", http.FileServer(http.FS(s.web)))
        return securityHeaders(mux)
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
        state, err := s.engine.State()
        if err != nil {
                writeError(w, http.StatusInternalServerError, err)
                return
        }
        writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleDispatch(w http.ResponseWriter, r *http.Request) {
        var req model.DispatchRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                writeError(w, http.StatusBadRequest, errors.New("некорректный запрос диспетчеризации"))
                return
        }
        state, err := s.engine.Dispatch(game.DispatchInput{OrderID: req.OrderID, RoverID: req.RoverID})
        if err != nil {
                writeError(w, statusForError(err), err)
                return
        }
        writeJSON(w, http.StatusAccepted, state)
}

func (s *Server) handleNextDay(w http.ResponseWriter, _ *http.Request) {
        state, err := s.engine.NextDay()
        if err != nil {
                writeError(w, statusForError(err), err)
                return
        }
        writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleReset(w http.ResponseWriter, _ *http.Request) {
        state, err := s.engine.Reset()
        if err != nil {
                writeError(w, http.StatusInternalServerError, err)
                return
        }
        writeJSON(w, http.StatusOK, state)
}

func securityHeaders(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("X-Content-Type-Options", "nosniff")
                w.Header().Set("Referrer-Policy", "no-referrer")
                w.Header().Set("Cache-Control", "no-cache")
                next.ServeHTTP(w, r)
        })
}

func statusForError(err error) int {
        msg := strings.TrimSpace(err.Error())
        switch {
        case strings.Contains(msg, "не найден"):
                return http.StatusNotFound
        case strings.HasPrefix(msg, "некорректный"):
                return http.StatusBadRequest
        default:
                return http.StatusUnprocessableEntity
        }
}

func writeJSON(w http.ResponseWriter, status int, value any) {
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        w.WriteHeader(status)
        if err := json.NewEncoder(w).Encode(value); err != nil {
                log.Printf("encode response: %v", err)
        }
}

func writeError(w http.ResponseWriter, status int, err error) {
        message := strings.TrimSpace(err.Error())
        if n, convErr := strconv.Atoi(message); convErr == nil {
                message = "ошибка " + strconv.Itoa(n)
        }
        writeJSON(w, status, map[string]string{"error": message})
}

func StaticFS(embedded fs.FS) fs.FS {
        if info, err := os.Stat("static"); err == nil && info.IsDir() {
                return os.DirFS("static")
        }
        sub, err := fs.Sub(embedded, "static")
        if err != nil {
                log.Fatal(err)
        }
        return sub
}
