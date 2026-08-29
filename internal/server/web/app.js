const token = document.querySelector('meta[name="panelpc-token"]').content;
const headers = { 'X-PanelPC-Token': token };
const cards = [];
let config;
let audioDevices = [];
let noticeTimer;

async function api(path, options = {}) {
  const response = await fetch(path, { ...options, headers: { ...headers, ...(options.headers || {}) } });
  if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
  if (response.status === 204) return null;
  return response.json();
}

function showNotice(message, success = false) {
  const notice = document.querySelector('#notice');
  notice.textContent = message;
  notice.classList.toggle('success', success);
  notice.classList.remove('hidden');
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => notice.classList.add('hidden'), 5000);
}

function toggleFields(card) {
  const turn = card.querySelector('.turn-kind').value;
  card.querySelector('.target-wrap').classList.toggle('hidden', !['app', 'obs_input'].includes(turn));
  card.querySelector('.device-target-wrap').classList.toggle('hidden', !['output_device', 'input_device'].includes(turn));
  card.querySelector('.percent-range').classList.toggle('hidden', ['none', 'obs_filter', 'shell'].includes(turn));
  card.querySelector('.filter-fields').classList.toggle('hidden', turn !== 'obs_filter');
  card.querySelector('.shell-turn-fields').classList.toggle('hidden', turn !== 'shell');
  const target = card.querySelector('.turn-target');
  target.placeholder = turn === 'app' ? 'Spotify, Firefox…' : 'Mic/Aux';
  target.removeAttribute('list');
  if (turn === 'app') target.setAttribute('list', 'audio-apps');
  if (turn === 'obs_input') target.setAttribute('list', 'obs-inputs');
  card.querySelector('.target-label').textContent = turn === 'app' ? 'Aplicación' : 'Entrada OBS';
  card.querySelector('.device-target-label').textContent = turn === 'output_device' ? 'Dispositivo de salida' : 'Dispositivo de entrada';
  populateDeviceSelect(card);
  const press = card.querySelector('.press-kind').value;
  card.querySelector('.press-target-wrap').classList.toggle('hidden', !['obs_scene', 'obs_toggle_input_mute', 'profile'].includes(press));
  const pressTarget = card.querySelector('.press-target');
  pressTarget.removeAttribute('list');
  if (press === 'profile') pressTarget.setAttribute('list', 'profile-names');
  if (press === 'obs_toggle_input_mute') pressTarget.setAttribute('list', 'obs-inputs');
  card.querySelector('.press-target-label').textContent = press === 'profile' ? 'Perfil' : press === 'obs_scene' ? 'Escena OBS' : 'Entrada OBS';
  card.querySelector('.shell-press-fields').classList.toggle('hidden', press !== 'shell');
}

function toggleLightingMode() {
  const mode = document.querySelector('#lighting-mode').value;
  const reactive = ['vu', 'spectrum'].includes(mode);
  document.querySelector('#dial-lighting-controls').classList.toggle('hidden', reactive);
  document.querySelector('#vu-controls').classList.toggle('hidden', !reactive);
  document.querySelector('#lighting-title').textContent = mode === 'spectrum' ? 'Espectro RGB' : mode === 'vu' ? 'VU de nivel RGB' : 'RGB por dial';
  document.querySelector('#reactive-help').textContent = mode === 'spectrum'
    ? 'Cada anillo muestra una banda: graves, medios bajos, medios altos y agudos. Se usa un único stream y la salida USB se limita a este rate.'
    : 'Los cuatro anillos forman una barra de nivel de izquierda a derecha. El capturador es continuo y la salida USB se limita a este rate.';
  document.body.classList.toggle('vu-mode', reactive);
  toggleVUSource();
}

function setLightingCollapsed(collapsed) {
  const panel = document.querySelector('.lighting-bar');
  const button = document.querySelector('#toggle-lighting');
  panel.classList.toggle('collapsed', collapsed);
  button.textContent = collapsed ? 'Mostrar' : 'Ocultar';
  button.setAttribute('aria-expanded', String(!collapsed));
}

function toggleVUSource() {
  const kind = document.querySelector('#vu-source-kind').value;
  document.querySelector('#vu-app-wrap').classList.toggle('hidden', kind !== 'app');
  document.querySelector('#vu-device-wrap').classList.toggle('hidden', !['output_device', 'input_device'].includes(kind));
  populateVUDeviceSelect();
}

function setDial(card, value) {
  const dial = card.querySelector('.dial');
  dial.style.setProperty('--value', `${Math.round(value / 255 * 270)}deg`);
  dial.querySelector('span').textContent = Math.round(value / 255 * 100);
}

function renderKnob(knob, index) {
  const card = document.querySelector('#knob-template').content.firstElementChild.cloneNode(true);
  card.dataset.index = index;
  card.querySelector('.knob-label').value = knob.label;
  card.querySelector('.knob-color').value = knob.light.color;
  card.querySelector('.knob-track').checked = knob.light.trackValue;
  card.querySelector('.dial').style.setProperty('--color', knob.light.color);
  card.querySelector('.turn-kind').value = knob.turn.kind;
  card.querySelector('.turn-target').value = knob.turn.target || '';
  card.dataset.audioTarget = knob.turn.target || '';
  const deviceTarget = card.querySelector('.device-target');
  if (knob.turn.target) deviceTarget.append(new Option(knob.turn.target, knob.turn.target));
  deviceTarget.value = knob.turn.target || '';
  card.querySelector('.turn-min').value = knob.turn.minPercent;
  card.querySelector('.turn-max').value = knob.turn.maxPercent;
  card.querySelector('.filter-source').value = knob.turn.source || '';
  card.querySelector('.filter-name').value = knob.turn.filter || '';
  card.querySelector('.filter-setting').value = knob.turn.setting || 'opacity';
  card.querySelector('.filter-min').value = knob.turn.minValue ?? 0;
  card.querySelector('.filter-max').value = knob.turn.maxValue ?? 1;
  card.querySelector('.turn-command').value = knob.turn.command || '';
  card.querySelector('.turn-rate').value = knob.turn.rateMs || 250;
  card.querySelector('.press-kind').value = knob.press.kind;
  card.querySelector('.press-target').value = knob.press.target || '';
  card.querySelector('.press-command').value = knob.press.command || '';
  card.querySelectorAll('select').forEach(el => el.addEventListener('change', () => toggleFields(card)));
  deviceTarget.addEventListener('change', () => { card.dataset.audioTarget = deviceTarget.value; });
  card.querySelector('.knob-color').addEventListener('input', event => card.querySelector('.dial').style.setProperty('--color', event.target.value));
  const filterSource = card.querySelector('.filter-source');
  filterSource.addEventListener('change', () => loadOBSFilters(filterSource.value));
  card.querySelector('.filter-name').addEventListener('focus', () => loadOBSFilters(filterSource.value));
  let lastTest = 0;
  card.querySelector('.test-range').addEventListener('input', event => {
    const value = Number(event.target.value);
    setDial(card, value);
    const now = performance.now();
    if (now - lastTest > 60) {
      lastTest = now;
      api('/api/test', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ kind: 'turn', knob: index, value }) }).catch(err => showNotice(err.message));
    }
  });
  card.querySelector('.test-click').addEventListener('click', () => {
    api('/api/test', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ kind: 'press', knob: index, value: 1 }) }).catch(err => showNotice(err.message));
  });
  toggleFields(card);
  document.querySelector('#knobs').append(card);
  cards.push(card);
}

function renderProfile(profile) {
  config.lighting = profile.lighting;
  config.knobs = profile.knobs;
  cards.length = 0;
  document.querySelector('#knobs').replaceChildren();
  profile.knobs.forEach(renderKnob);

  const lighting = profile.lighting;
  const vu = lighting.vu;
  document.querySelector('#lighting-mode').value = lighting.mode;
  const brightness = document.querySelector('#global-brightness');
  brightness.value = lighting.globalBrightness;
  document.querySelector('#global-brightness-value').textContent = `${brightness.value}%`;
  document.querySelector('#vu-source-kind').value = vu.sourceKind;
  document.querySelector('#vu-app-target').value = vu.sourceKind === 'app' ? vu.target : '';
  const vuDeviceTarget = document.querySelector('#vu-device-target');
  vuDeviceTarget.dataset.target = ['output_device', 'input_device'].includes(vu.sourceKind) ? vu.target : '';
  document.querySelector('#vu-min-color').value = vu.minColor;
  document.querySelector('#vu-max-color').value = vu.maxColor;
  const vuBrightness = document.querySelector('#vu-brightness');
  vuBrightness.value = vu.brightness;
  document.querySelector('#vu-brightness-value').textContent = `${vuBrightness.value}%`;
  document.querySelector('#vu-min-db').value = vu.minDb;
  document.querySelector('#vu-max-db').value = vu.maxDb;
  document.querySelector('#vu-fps').value = vu.fps;
  toggleLightingMode();
  refreshProfileOptions();
  if (audioDevices.length) {
    cards.forEach(populateDeviceSelect);
    populateVUDeviceSelect();
  }
}

function refreshProfileOptions() {
  const names = Object.keys(config.profiles || {}).sort((a, b) => a.localeCompare(b));
  const select = document.querySelector('#active-profile');
  select.replaceChildren(...names.map(name => new Option(name, name)));
  select.value = config.activeProfile;
  const datalist = document.querySelector('#profile-names');
  datalist.replaceChildren(...names.map(name => {
    const option = document.createElement('option');
    option.value = name;
    return option;
  }));
}

function readConfig() {
  const vuKind = document.querySelector('#vu-source-kind').value;
  const vuTarget = vuKind === 'app'
    ? document.querySelector('#vu-app-target').value.trim()
    : ['output_device', 'input_device'].includes(vuKind) ? document.querySelector('#vu-device-target').value : '';
  const result = {
    version: 4,
    obs: { url: document.querySelector('#obs-url').value.trim(), password: document.querySelector('#obs-password').value },
    lighting: {
      globalBrightness: Number(document.querySelector('#global-brightness').value),
      mode: document.querySelector('#lighting-mode').value,
      vu: {
        sourceKind: vuKind,
        target: vuTarget,
        minColor: document.querySelector('#vu-min-color').value,
        maxColor: document.querySelector('#vu-max-color').value,
        brightness: Number(document.querySelector('#vu-brightness').value),
        minDb: Number(document.querySelector('#vu-min-db').value),
        maxDb: Number(document.querySelector('#vu-max-db').value),
        fps: Number(document.querySelector('#vu-fps').value),
      },
    },
    knobs: cards.map(card => {
      const turnKind = card.querySelector('.turn-kind').value;
      const target = ['output_device', 'input_device'].includes(turnKind)
        ? card.querySelector('.device-target').value
        : card.querySelector('.turn-target').value.trim();
      return ({
      label: card.querySelector('.knob-label').value.trim(),
      light: { color: card.querySelector('.knob-color').value, trackValue: card.querySelector('.knob-track').checked },
      turn: {
        kind: turnKind,
        target,
        minPercent: Number(card.querySelector('.turn-min').value),
        maxPercent: Number(card.querySelector('.turn-max').value),
        source: card.querySelector('.filter-source').value.trim(),
        filter: card.querySelector('.filter-name').value.trim(),
        setting: card.querySelector('.filter-setting').value.trim(),
        minValue: Number(card.querySelector('.filter-min').value),
        maxValue: Number(card.querySelector('.filter-max').value),
        command: card.querySelector('.turn-command').value,
        rateMs: Number(card.querySelector('.turn-rate').value),
      },
      press: {
        kind: card.querySelector('.press-kind').value,
        target: card.querySelector('.press-target').value.trim(),
        command: card.querySelector('.press-command').value,
      },
      });
    }),
  };
  result.activeProfile = config.activeProfile;
  result.profiles = { ...(config.profiles || {}) };
  result.profiles[result.activeProfile] = { lighting: result.lighting, knobs: result.knobs };
  return result;
}

async function switchProfile(name) {
  if (!name || name === config.activeProfile || !config.profiles[name]) return;
  config = readConfig();
  config.activeProfile = name;
  renderProfile(config.profiles[name]);
  await save();
}

async function createProfile() {
  const rawName = window.prompt('Nombre del nuevo perfil:');
  if (rawName === null) return;
  const name = rawName.trim();
  if (!name) {
    showNotice('El nombre del perfil no puede estar vacío.');
    return;
  }
  config = readConfig();
  if (config.profiles[name]) {
    showNotice(`Ya existe el perfil “${name}”.`);
    return;
  }
  config.profiles[name] = { lighting: config.lighting, knobs: config.knobs };
  config.activeProfile = name;
  renderProfile(config.profiles[name]);
  await save();
}

async function save() {
  const button = document.querySelector('#save');
  button.disabled = true;
  try {
    config = readConfig();
    await api('/api/config', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config) });
    setLightingCollapsed(true);
    showNotice('Configuración guardada.', true);
  } catch (err) {
    showNotice(err.message);
  } finally {
    button.disabled = false;
  }
}

async function refreshStatus() {
  try {
    const status = await api('/api/status');
    if (status.activeProfile && status.activeProfile !== config.activeProfile) {
      config = await api('/api/config');
      renderProfile({ lighting: config.lighting, knobs: config.knobs });
      showNotice(`Perfil “${config.activeProfile}” activado.`, true);
    }
    const pill = document.querySelector('#device-pill');
    pill.classList.toggle('connected', status.device.connected);
    pill.classList.toggle('error', Boolean(status.device.error));
    pill.querySelector('b').textContent = status.device.connected ? (status.device.device || 'PCPanel conectado') : status.device.error ? 'Error de dispositivo' : 'PCPanel desconectado';
    pill.title = status.device.error || '';
    const lightingMode = document.querySelector('#lighting-mode').value;
    document.querySelector('#last-event').textContent = lightingMode === 'spectrum'
      ? `Espectro · ${(status.engine.spectrum || [0, 0, 0, 0]).map(value => Math.round(value * 100)).join(' · ')}%`
      : lightingMode === 'vu' ? `VU ${Math.round((status.engine.vuLevel || 0) * 100)}%` : status.engine.lastEvent || 'Sin eventos todavía';
    status.engine.values.forEach((value, index) => {
      setDial(cards[index], value);
      const level = lightingMode === 'spectrum'
        ? (status.engine.spectrum || [0, 0, 0, 0])[index]
        : lightingMode === 'vu' ? Math.max(0, Math.min(1, (status.engine.vuLevel || 0) * 4 - index)) : value / 255;
      document.querySelectorAll('#levels i')[index].style.setProperty('--level', `${level * 100}%`);
    });
    if (status.engine.lastError) document.querySelector('#last-event').textContent = status.engine.lastError;
    if (status.engine.vuError) document.querySelector('#last-event').textContent = `VU: ${status.engine.vuError}`;
  } catch (_) {}
}

async function loadApps() {
  try {
    const apps = await api('/api/audio/apps');
    const datalist = document.querySelector('#audio-apps');
    datalist.replaceChildren(...apps.map(app => {
      const option = document.createElement('option');
      option.value = app.name;
      return option;
    }));
  } catch (_) {}
}

function populateDeviceSelect(card) {
  const kind = card.querySelector('.turn-kind').value;
  const wantedKind = kind === 'output_device' ? 'output' : kind === 'input_device' ? 'input' : '';
  const select = card.querySelector('.device-target');
  const previous = select.value || card.dataset.audioTarget || '';
  const matching = audioDevices.filter(device => device.kind === wantedKind);
  select.replaceChildren();
  const placeholder = new Option(matching.length ? 'Selecciona un dispositivo' : 'No hay dispositivos disponibles', '');
  placeholder.disabled = matching.length > 0;
  select.append(placeholder);
  matching.forEach(device => {
    const state = device.state === 'running' ? ' · activo' : '';
    select.append(new Option(`${device.name}${state}`, device.id));
  });
  if (matching.some(device => device.id === previous)) {
    select.value = previous;
    card.dataset.audioTarget = previous;
  }
}

function populateVUDeviceSelect() {
  const kindElement = document.querySelector('#vu-source-kind');
  const select = document.querySelector('#vu-device-target');
  if (!kindElement || !select) return;
  const kind = kindElement.value;
  const wantedKind = kind === 'output_device' ? 'output' : kind === 'input_device' ? 'input' : '';
  const previous = select.value || select.dataset.target || '';
  const matching = audioDevices.filter(device => device.kind === wantedKind);
  select.replaceChildren();
  const placeholder = new Option(matching.length ? 'Selecciona un dispositivo' : 'No hay dispositivos disponibles', '');
  placeholder.disabled = matching.length > 0;
  select.append(placeholder);
  matching.forEach(device => {
    const state = device.state === 'running' ? ' · activo' : '';
    select.append(new Option(`${device.name}${state}`, device.id));
  });
  if (matching.some(device => device.id === previous)) select.value = previous;
}

async function loadAudioDevices() {
  try {
    audioDevices = await api('/api/audio/devices');
    cards.forEach(populateDeviceSelect);
    populateVUDeviceSelect();
  } catch (_) {}
}

async function loadOBSInputs() {
  try {
    const inputs = await api('/api/obs/inputs');
    const datalist = document.querySelector('#obs-inputs');
    datalist.replaceChildren(...inputs.map(name => {
      const option = document.createElement('option');
      option.value = name;
      return option;
    }));
  } catch (_) {}
}

async function loadOBSFilters(source) {
  if (!source.trim()) return;
  try {
    const filters = await api(`/api/obs/filters?source=${encodeURIComponent(source.trim())}`);
    const datalist = document.querySelector('#obs-filters');
    datalist.replaceChildren(...filters.map(name => {
      const option = document.createElement('option');
      option.value = name;
      return option;
    }));
  } catch (_) {}
}

async function init() {
  try {
    config = await api('/api/config');
    document.querySelector('#global-brightness').addEventListener('input', event => {
      document.querySelector('#global-brightness-value').textContent = `${event.target.value}%`;
    });
    document.querySelector('#lighting-mode').addEventListener('change', toggleLightingMode);
    document.querySelector('#toggle-lighting').addEventListener('click', () => {
      setLightingCollapsed(!document.querySelector('.lighting-bar').classList.contains('collapsed'));
    });
    document.querySelector('#vu-source-kind').addEventListener('change', toggleVUSource);
    const vuDeviceTarget = document.querySelector('#vu-device-target');
    vuDeviceTarget.addEventListener('change', () => { vuDeviceTarget.dataset.target = vuDeviceTarget.value; });
    const vuBrightness = document.querySelector('#vu-brightness');
    vuBrightness.addEventListener('input', event => {
      document.querySelector('#vu-brightness-value').textContent = `${event.target.value}%`;
    });
    renderProfile({ lighting: config.lighting, knobs: config.knobs });
    document.querySelector('#obs-url').value = config.obs.url;
    document.querySelector('#obs-password').value = config.obs.password;
    const levels = document.querySelector('#levels');
    for (let i = 0; i < 4; i++) levels.append(document.createElement('i'));
    document.querySelector('#save').addEventListener('click', save);
    document.querySelector('#active-profile').addEventListener('change', event => {
      switchProfile(event.target.value).catch(err => showNotice(err.message));
    });
    document.querySelector('#new-profile').addEventListener('click', () => {
      createProfile().catch(err => showNotice(err.message));
    });
    document.querySelector('#test-obs').addEventListener('click', async event => {
      await save();
      event.target.disabled = true;
      try {
        await api('/api/obs/test', { method: 'POST' });
        showNotice('OBS respondió correctamente.', true);
      } catch (err) {
        showNotice(err.message);
      } finally {
        event.target.disabled = false;
      }
    });
    await loadApps();
    await loadAudioDevices();
    await loadOBSInputs();
    await refreshStatus();
    setInterval(refreshStatus, 500);
    setInterval(loadApps, 10000);
    setInterval(loadAudioDevices, 10000);
    setInterval(loadOBSInputs, 15000);
  } catch (err) {
    showNotice(`No se pudo iniciar: ${err.message}`);
  }
}

init();
