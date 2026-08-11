import { createApp, nextTick } from '/assets/vue.esm-browser.prod.js';

const STAGE_SIZE = 1600;

const SOUND_FILES = Object.freeze({
  uiClick: '/sounds/ui-click.mp3',
  uiSelect: '/sounds/ui-select.mp3',
  routeConfirm: '/sounds/route-confirm.mp3',
  roverLaunch: '/sounds/rover-launch.mp3',
  success: '/sounds/delivery-success.mp3',
  failure: '/sounds/delivery-failed.mp3',
  dayAdvance: '/sounds/day-advance.mp3',
  warning: '/sounds/warning.mp3',
  ambient: '/sounds/lunar-ambience.mp3',
});

let ambientTrack = null;
const activeSounds = new Set();

createApp({
  data() {
    return {
      state: null,
      loading: true,
      selectedOrderId: null,
      selectedRoverId: null,
      orderFilter: 'active',
      dispatching: false,
      rulesOpen: false,
      audioEnabled: localStorage.getItem('luna-audio-muted') !== '1',
      audioUnlockHandler: null,
      now: Date.now(),
      clockOffset: 0,
      toast: { show: false, type: 'success', message: '' },
      toastTimer: null,
      pollTimer: null,
      tickTimer: null,
      lastEventId: 0,
      map: {
        x: 0,
        y: 0,
        scale: 0.55,
        minScale: 0.45,
        maxScale: 2.2,
        dragging: false,
        vx: 0,
        vy: 0,
        lastX: 0,
        lastY: 0,
        lastT: 0,
      },
      pointers: new Map(),
      pinch: null,
      inertiaFrame: 0,
      mapAnimation: 0,
    };
  },

  computed: {
    availableOrders() {
      return this.state.orders.filter(order => order.status === 'available');
    },
    filteredOrders() {
      if (this.orderFilter === 'all') return this.state.orders;
      return this.state.orders.filter(order => ['available', 'delivering'].includes(order.status));
    },
    selectedOrder() {
      return this.state.orders.find(order => order.id === this.selectedOrderId) || null;
    },
    selectedRover() {
      return this.state.rovers.find(rover => rover.id === this.selectedRoverId) || null;
    },
    selectedFeasibility() {
      if (!this.selectedOrder || !this.selectedRover) {
        return { ok: false, reason: 'Сначала выберите ровер', cost: 0, duration: 0 };
      }
      return this.feasibility(this.selectedOrder, this.selectedRover);
    },
    feasibleRovers() {
      if (!this.selectedOrder) return [];
      return this.state.rovers.filter(rover => this.feasibility(this.selectedOrder, rover).ok);
    },
    activeDeliveries() {
      return this.state.deliveries.filter(delivery => delivery.status === 'enroute');
    },
    hasActive() {
      return this.activeDeliveries.length > 0;
    },
    expectedReward() {
      if (!this.selectedOrder) return 0;
      if (this.state.game.day < this.selectedOrder.deadlineDay) {
        return Math.round(this.selectedOrder.reward * 1.1);
      }
      return this.selectedOrder.reward;
    },
    stageStyle() {
      return {
        transform: `translate3d(${this.map.x}px, ${this.map.y}px, 0) scale(${this.map.scale})`,
      };
    },
  },

  async mounted() {
    await this.loadState(true);
    this.tickTimer = window.setInterval(() => {
      this.now = Date.now() + this.clockOffset;
    }, 80);
    this.pollTimer = window.setInterval(() => this.loadState(false), 1100);
    window.addEventListener('resize', this.onResize, { passive: true });
    this.audioUnlockHandler = () => this.startAmbient();
    window.addEventListener('pointerdown', this.audioUnlockHandler, { once: true, passive: true });
  },

  beforeUnmount() {
    window.clearInterval(this.tickTimer);
    window.clearInterval(this.pollTimer);
    window.clearTimeout(this.toastTimer);
    window.cancelAnimationFrame(this.inertiaFrame);
    window.cancelAnimationFrame(this.mapAnimation);
    window.removeEventListener('resize', this.onResize);
    if (this.audioUnlockHandler) window.removeEventListener('pointerdown', this.audioUnlockHandler);
    if (ambientTrack) ambientTrack.pause();
    activeSounds.forEach(sound => sound.pause());
    activeSounds.clear();
  },

  methods: {
    playSound(name, volume = 0.45) {
      if (!this.audioEnabled || !SOUND_FILES[name]) return;
      const sound = new Audio(SOUND_FILES[name]);
      sound.preload = 'auto';
      sound.volume = Math.max(0, Math.min(1, volume));
      activeSounds.add(sound);
      const release = () => activeSounds.delete(sound);
      sound.addEventListener('ended', release, { once: true });
      sound.addEventListener('error', release, { once: true });
      sound.play().catch(release);
    },

    startAmbient() {
      if (!this.audioEnabled) return;
      if (!ambientTrack) {
        ambientTrack = new Audio(SOUND_FILES.ambient);
        ambientTrack.loop = true;
        ambientTrack.preload = 'auto';
        ambientTrack.volume = 0.12;
        ambientTrack.addEventListener('error', () => { ambientTrack = null; }, { once: true });
      }
      ambientTrack.play().catch(() => {});
    },

    toggleAudio() {
      this.audioEnabled = !this.audioEnabled;
      localStorage.setItem('luna-audio-muted', this.audioEnabled ? '0' : '1');
      if (this.audioEnabled) {
        this.playSound('uiClick', 0.28);
        this.startAmbient();
        this.notify('Аудиоканал включён.', 'success');
      } else {
        if (ambientTrack) ambientTrack.pause();
        activeSounds.forEach(sound => sound.pause());
        activeSounds.clear();
        this.notify('Аудиоканал выключен.', 'success');
      }
    },
    async loadState(initial = false) {
      try {
        const response = await fetch('/api/state', { cache: 'no-store' });
        if (!response.ok) throw new Error('Диспетчерский сервер недоступен');
        const data = await response.json();
        const previousLatest = this.lastEventId;
        this.state = data;
        this.clockOffset = data.serverTime - Date.now();
        this.now = data.serverTime;
        const latest = data.events[0];
        if (!initial && latest && latest.id > previousLatest && ['success', 'failure'].includes(latest.kind)) {
          const failed = latest.kind === 'failure';
          this.playSound(failed ? 'failure' : 'success', failed ? 0.58 : 0.52);
          this.notify(latest.message, failed ? 'error' : 'success');
        }
        if (latest) this.lastEventId = Math.max(this.lastEventId, latest.id);
        if (initial) {
          this.loading = false;
          await nextTick();
          this.initMap();
        }
      } catch (error) {
        if (initial) this.loading = false;
        this.notify(error.message || 'Ошибка синхронизации', 'error');
      }
    },

    async api(path, body) {
      const response = await fetch(path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: body ? JSON.stringify(body) : undefined,
      });
      let data = {};
      try { data = await response.json(); } catch (_) { /* no-op */ }
      if (!response.ok) throw new Error(data.error || 'Операция отклонена');
      return data;
    },

    selectOrder(id) {
      if (this.selectedOrderId !== id) this.playSound('uiSelect', 0.24);
      this.selectedOrderId = id;
      if (this.selectedRoverId && !this.state.rovers.some(r => r.id === this.selectedRoverId)) {
        this.selectedRoverId = null;
      }
    },

    selectRover(id) {
      if (this.selectedRoverId !== id) this.playSound('uiSelect', 0.22);
      this.selectedRoverId = id;
    },

    async dispatch() {
      if (!this.selectedOrder || !this.selectedRover || !this.selectedFeasibility.ok || this.dispatching) return;
      this.playSound('routeConfirm', 0.42);
      this.dispatching = true;
      try {
        const data = await this.api('/api/dispatch', {
          orderId: this.selectedOrder.id,
          roverId: this.selectedRover.id,
        });
        this.state = data;
        this.clockOffset = data.serverTime - Date.now();
        this.lastEventId = data.events[0]?.id || this.lastEventId;
        window.setTimeout(() => this.playSound('roverLaunch', 0.56), 180);
        this.notify('Маршрут принят. Ровер покинул базу.', 'success');
      } catch (error) {
        this.playSound('warning', 0.48);
        this.notify(error.message, 'error');
      } finally {
        this.dispatching = false;
      }
    },

    async advanceDay() {
      if (this.hasActive || this.state.game.finished) return;
      try {
        const data = await this.api('/api/next-day');
        this.state = data;
        this.clockOffset = data.serverTime - Date.now();
        this.lastEventId = data.events[0]?.id || this.lastEventId;
        this.playSound('dayAdvance', 0.46);
        if (!data.game.finished) this.notify(`Сутки ${data.game.day}: роверы прошли сервисный цикл.`, 'success');
      } catch (error) {
        this.playSound('warning', 0.48);
        this.notify(error.message, 'error');
      }
    },

    async resetGame() {
      this.playSound('uiClick', 0.28);
      try {
        const data = await this.api('/api/reset');
        this.state = data;
        this.selectedOrderId = null;
        this.selectedRoverId = null;
        this.orderFilter = 'active';
        this.lastEventId = data.events[0]?.id || 0;
        this.clockOffset = data.serverTime - Date.now();
        this.notify('Новая экспедиция подготовлена.', 'success');
        await nextTick();
        this.resetMap();
      } catch (error) {
        this.notify(error.message, 'error');
      }
    },

    feasibility(order, rover) {
      if (!order || !rover) return { ok: false, reason: 'Нет данных', cost: 0, duration: 0 };
      const cost = Math.ceil((((order.distance * 0.45 + order.weight * 0.06) / rover.efficiency) + order.risk * 0.08) * 10) / 10;
      const routeFactor = Math.max(0.45, order.speedFactor);
      const duration = Math.max(7, Math.ceil(5 + order.distance / (rover.speed * routeFactor * 20) + order.weight / 95 + order.risk / 35));
      if (order.status !== 'available') return { ok: false, reason: 'Заказ недоступен', cost, duration };
      if (rover.status !== 'idle') return { ok: false, reason: 'Ровер уже в рейсе', cost, duration };
      if (rover.capacity < order.weight) return { ok: false, reason: `Лимит ${rover.capacity} кг — не хватает ${Math.ceil(order.weight - rover.capacity)} кг`, cost, duration };
      if (rover.battery + 0.001 < cost) return { ok: false, reason: `Нужно ${cost}% — доступно ${Math.round(rover.battery)}%`, cost, duration };
      if (this.state.game.day > order.deadlineDay) return { ok: false, reason: 'Окно доставки закрыто', cost, duration };
      return { ok: true, reason: '', cost, duration };
    },

    deadlineText(order) {
      if (order.status === 'delivered') return 'ДОСТАВЛЕН';
      if (order.status === 'failed') return 'ПРОВАЛ';
      if (order.status === 'expired') return 'ПРОСРОЧЕН';
      if (order.status === 'delivering') return 'В ПУТИ';
      const days = order.deadlineDay - this.state.game.day;
      if (days <= 0) return 'СЕГОДНЯ';
      return `ДЕНЬ ${order.deadlineDay}`;
    },

    urgencyClass(order) {
      if (['failed', 'expired'].includes(order.status)) return 'closed';
      if (order.status === 'available' && order.deadlineDay <= this.state.game.day) return 'urgent';
      return '';
    },

    riskClass(risk) {
      if (risk >= 40) return 'risk-high';
      if (risk >= 20) return 'risk-medium';
      return 'risk-low';
    },

    batteryClass(value) {
      if (value < 30) return 'low';
      if (value < 55) return 'medium';
      return 'high';
    },

    statusLabel(status) {
      return {
        delivering: 'ДОСТАВКА ВЫПОЛНЯЕТСЯ',
        delivered: 'ДОСТАВКА ЗАВЕРШЕНА',
        failed: 'ДОСТАВКА ПРОВАЛЕНА',
        expired: 'ОКНО ЗАКРЫТО',
      }[status] || 'ЗАКАЗ НЕДОСТУПЕН';
    },

    closedOrderText(status) {
      return {
        delivering: 'Ровер движется по подтверждённому маршруту и вернётся на базу автоматически.',
        delivered: 'Груз принят получателем. Выплата и очки уже зачислены на баланс базы.',
        failed: 'Рельеф сорвал миссию. Заряд потрачен, выплата потеряна, рейтинг снижен.',
        expired: 'Дедлайн прошёл. Этот груз больше нельзя отправить.',
      }[status] || 'Операции с заказом заблокированы.';
    },

    activeCountdown(delivery) {
      const left = Math.max(0, delivery.completesAt - this.now);
      return `${Math.ceil(left / 1000)} С`;
    },

    formatNumber(value) {
      return new Intl.NumberFormat('ru-RU').format(Math.round(value));
    },

    pad(value) {
      return String(value).padStart(2, '0');
    },

    notify(message, type = 'success') {
      window.clearTimeout(this.toastTimer);
      this.toast = { show: true, type, message };
      this.toastTimer = window.setTimeout(() => { this.toast.show = false; }, 3600);
    },

    positionStyle(x, y) {
      return { left: `${x}%`, top: `${y}%` };
    },

    routeControl(targetX, targetY) {
      const bx = this.state.game.baseX;
      const by = this.state.game.baseY;
      const dx = targetX - bx;
      const dy = targetY - by;
      const length = Math.max(1, Math.hypot(dx, dy));
      const bend = Math.min(6, 2.7 + length * 0.04);
      return {
        x: (bx + targetX) / 2 - (dy / length) * bend,
        y: (by + targetY) / 2 + (dx / length) * bend,
      };
    },

    routePath(order) {
      const c = this.routeControl(order.x, order.y);
      return `M ${this.state.game.baseX} ${this.state.game.baseY} Q ${c.x.toFixed(2)} ${c.y.toFixed(2)} ${order.x} ${order.y}`;
    },

    deliveryPath(delivery) {
      const c = this.routeControl(delivery.targetX, delivery.targetY);
      return `M ${this.state.game.baseX} ${this.state.game.baseY} Q ${c.x.toFixed(2)} ${c.y.toFixed(2)} ${delivery.targetX} ${delivery.targetY}`;
    },

    quadraticPoint(t, tx, ty) {
      const bx = this.state.game.baseX;
      const by = this.state.game.baseY;
      const c = this.routeControl(tx, ty);
      const inv = 1 - t;
      return {
        x: inv * inv * bx + 2 * inv * t * c.x + t * t * tx,
        y: inv * inv * by + 2 * inv * t * c.y + t * t * ty,
      };
    },

    roverPositionStyle(rover, index) {
      const delivery = this.activeDeliveries.find(item => item.roverId === rover.id);
      if (!delivery) {
        const offsets = [{ x: -1.4, y: .9 }, { x: 1.5, y: 1.1 }, { x: .1, y: -1.2 }];
        return this.positionStyle(this.state.game.baseX + offsets[index].x, this.state.game.baseY + offsets[index].y);
      }
      const raw = Math.max(0, Math.min(1, (this.now - delivery.startedAt) / (delivery.completesAt - delivery.startedAt)));
      let t;
      if (raw <= 0.50) t = raw / 0.50;
      else if (raw <= 0.58) t = 1;
      else t = 1 - (raw - 0.58) / 0.42;
      t = Math.max(0, Math.min(1, t));
      const eased = t * t * (3 - 2 * t);
      const point = this.quadraticPoint(eased, delivery.targetX, delivery.targetY);
      return this.positionStyle(point.x, point.y);
    },

    initMap() {
      const viewport = this.$refs.mapViewport;
      if (!viewport) return;
      const rect = viewport.getBoundingClientRect();
      const fit = Math.max(rect.width / STAGE_SIZE, rect.height / STAGE_SIZE) * 1.04;
      this.map.minScale = Math.max(0.28, fit);
      this.map.maxScale = this.map.minScale * 3.7;
      this.map.scale = this.map.minScale;
      this.map.x = (rect.width - STAGE_SIZE * this.map.scale) / 2;
      this.map.y = (rect.height - STAGE_SIZE * this.map.scale) / 2;
      this.clampMap();
    },

    onResize() {
      if (!this.$refs.mapViewport) return;
      const ratio = this.map.scale / this.map.minScale;
      const rect = this.$refs.mapViewport.getBoundingClientRect();
      this.map.minScale = Math.max(0.28, Math.max(rect.width / STAGE_SIZE, rect.height / STAGE_SIZE) * 1.04);
      this.map.maxScale = this.map.minScale * 3.7;
      this.map.scale = Math.min(this.map.maxScale, Math.max(this.map.minScale, this.map.minScale * ratio));
      this.clampMap();
    },

    onWheel(event) {
      const rect = this.$refs.mapViewport.getBoundingClientRect();
      const px = event.clientX - rect.left;
      const py = event.clientY - rect.top;
      const factor = Math.exp(-event.deltaY * 0.00125);
      this.zoomAt(px, py, this.map.scale * factor);
    },

    zoomAt(px, py, requestedScale) {
      const oldScale = this.map.scale;
      const newScale = Math.min(this.map.maxScale, Math.max(this.map.minScale, requestedScale));
      if (Math.abs(newScale - oldScale) < 0.0001) return;
      const worldX = (px - this.map.x) / oldScale;
      const worldY = (py - this.map.y) / oldScale;
      this.map.scale = newScale;
      this.map.x = px - worldX * newScale;
      this.map.y = py - worldY * newScale;
      this.clampMap();
    },

    zoomBy(factor) {
      const rect = this.$refs.mapViewport.getBoundingClientRect();
      this.zoomAt(rect.width / 2, rect.height / 2, this.map.scale * factor);
    },

    onPointerDown(event) {
      if (event.pointerType === 'mouse' && event.button !== 0) return;
      window.cancelAnimationFrame(this.inertiaFrame);
      window.cancelAnimationFrame(this.mapAnimation);
      this.$refs.mapViewport.setPointerCapture?.(event.pointerId);
      this.pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
      this.map.dragging = true;
      this.map.lastX = event.clientX;
      this.map.lastY = event.clientY;
      this.map.lastT = performance.now();
      this.map.vx = 0;
      this.map.vy = 0;
      if (this.pointers.size === 2) this.beginPinch();
    },

    beginPinch() {
      const [a, b] = [...this.pointers.values()];
      const rect = this.$refs.mapViewport.getBoundingClientRect();
      const centerX = (a.x + b.x) / 2 - rect.left;
      const centerY = (a.y + b.y) / 2 - rect.top;
      this.pinch = {
        distance: Math.hypot(a.x - b.x, a.y - b.y),
        scale: this.map.scale,
        worldX: (centerX - this.map.x) / this.map.scale,
        worldY: (centerY - this.map.y) / this.map.scale,
      };
    },

    onPointerMove(event) {
      if (!this.pointers.has(event.pointerId)) return;
      this.pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
      if (this.pointers.size >= 2) {
        if (!this.pinch) this.beginPinch();
        const [a, b] = [...this.pointers.values()];
        const rect = this.$refs.mapViewport.getBoundingClientRect();
        const centerX = (a.x + b.x) / 2 - rect.left;
        const centerY = (a.y + b.y) / 2 - rect.top;
        const distance = Math.max(1, Math.hypot(a.x - b.x, a.y - b.y));
        const scale = Math.min(this.map.maxScale, Math.max(this.map.minScale, this.pinch.scale * distance / this.pinch.distance));
        this.map.scale = scale;
        this.map.x = centerX - this.pinch.worldX * scale;
        this.map.y = centerY - this.pinch.worldY * scale;
        this.clampMap();
        return;
      }
      const now = performance.now();
      const dt = Math.max(8, now - this.map.lastT);
      const dx = event.clientX - this.map.lastX;
      const dy = event.clientY - this.map.lastY;
      this.map.x += dx;
      this.map.y += dy;
      this.map.vx = this.map.vx * 0.35 + (dx / dt) * 0.65;
      this.map.vy = this.map.vy * 0.35 + (dy / dt) * 0.65;
      this.map.lastX = event.clientX;
      this.map.lastY = event.clientY;
      this.map.lastT = now;
      this.clampMap();
    },

    onPointerUp(event) {
      if (!this.pointers.has(event.pointerId)) return;
      this.pointers.delete(event.pointerId);
      this.pinch = null;
      if (this.pointers.size === 1) {
        const point = [...this.pointers.values()][0];
        this.map.lastX = point.x;
        this.map.lastY = point.y;
        this.map.lastT = performance.now();
        return;
      }
      this.map.dragging = false;
      this.startInertia();
    },

    startInertia() {
      let last = performance.now();
      const frame = (now) => {
        const dt = Math.min(32, now - last);
        last = now;
        this.map.x += this.map.vx * dt;
        this.map.y += this.map.vy * dt;
        this.map.vx *= Math.pow(0.91, dt / 16.7);
        this.map.vy *= Math.pow(0.91, dt / 16.7);
        this.clampMap();
        if (Math.hypot(this.map.vx, this.map.vy) > 0.012) {
          this.inertiaFrame = requestAnimationFrame(frame);
        }
      };
      this.inertiaFrame = requestAnimationFrame(frame);
    },

    clampMap() {
      const viewport = this.$refs.mapViewport;
      if (!viewport) return;
      const rect = viewport.getBoundingClientRect();
      const width = STAGE_SIZE * this.map.scale;
      const height = STAGE_SIZE * this.map.scale;
      if (width <= rect.width) this.map.x = (rect.width - width) / 2;
      else this.map.x = Math.min(0, Math.max(rect.width - width, this.map.x));
      if (height <= rect.height) this.map.y = (rect.height - height) / 2;
      else this.map.y = Math.min(0, Math.max(rect.height - height, this.map.y));
    },

    resetMap() {
      const rect = this.$refs.mapViewport.getBoundingClientRect();
      this.animateMap({
        scale: this.map.minScale,
        x: (rect.width - STAGE_SIZE * this.map.minScale) / 2,
        y: (rect.height - STAGE_SIZE * this.map.minScale) / 2,
      });
    },

    focusBase() {
      const rect = this.$refs.mapViewport.getBoundingClientRect();
      const scale = Math.min(this.map.maxScale, this.map.minScale * 1.7);
      this.animateMap({
        scale,
        x: rect.width / 2 - (this.state.game.baseX / 100 * STAGE_SIZE) * scale,
        y: rect.height / 2 - (this.state.game.baseY / 100 * STAGE_SIZE) * scale,
      });
    },

    animateMap(target) {
      window.cancelAnimationFrame(this.mapAnimation);
      const start = { x: this.map.x, y: this.map.y, scale: this.map.scale };
      const began = performance.now();
      const duration = 420;
      const frame = (now) => {
        const raw = Math.min(1, (now - began) / duration);
        const t = 1 - Math.pow(1 - raw, 3);
        this.map.x = start.x + (target.x - start.x) * t;
        this.map.y = start.y + (target.y - start.y) * t;
        this.map.scale = start.scale + (target.scale - start.scale) * t;
        this.clampMap();
        if (raw < 1) this.mapAnimation = requestAnimationFrame(frame);
      };
      this.mapAnimation = requestAnimationFrame(frame);
    },
  },
}).mount('#app');
