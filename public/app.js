const fileList = document.querySelector('#file-list');
const status = document.querySelector('#status');
const uploadDialog = document.querySelector('#upload-dialog');
const phoneDialog = document.querySelector('#phone-dialog');
const uploadForm = document.querySelector('#upload-form');
const submitUpload = document.querySelector('#submit-upload');
const lifetime = document.querySelector('#lifetime');
const lifetimeLabel = document.querySelector('#lifetime-label');
const fileTemplate = document.querySelector('#file-template');
const qrPanel = document.querySelector('#qr-panel');

let files = [];

function formatSize(bytes) {
  if (bytes < 1000) return `${bytes} B`;
  const units = ['kB', 'MB', 'GB', 'TB'];
  let value = bytes / 1000;
  let unit = units[0];
  for (let index = 1; value >= 1000 && index < units.length; index += 1) {
    value /= 1000;
    unit = units[index];
  }
  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${unit}`;
}

function formatLifetime(minutes) {
  if (minutes < 60) return `${minutes} minuter`;
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes ? `${hours} tim ${remainingMinutes} min` : `${hours} ${hours === 1 ? 'timme' : 'timmar'}`;
}

function formatRemaining(expiresAt) {
  const remainingMinutes = Math.max(0, Math.ceil((Date.parse(expiresAt) - Date.now()) / 60_000));
  if (remainingMinutes === 0) return 'snart';
  return formatLifetime(remainingMinutes);
}

function setStatus(message = '', isError = false) {
  status.textContent = message;
  status.classList.toggle('error', isError);
}

function configureDownloadAction(link, file) {
  link.textContent = 'Hämta';
  link.href = `/api/files/${encodeURIComponent(file.id)}/download`;
  link.setAttribute('download', '');
}

function renderFiles() {
  fileList.replaceChildren();

  if (files.length === 0) {
    const empty = document.createElement('div');
    empty.className = 'empty-state';
    empty.innerHTML = '<strong>Inga filer ännu</strong>Ladda upp den första filen';
    fileList.append(empty);
    return;
  }

  for (const file of files) {
    const card = fileTemplate.content.cloneNode(true);
    const article = card.querySelector('.file-card');
    article.dataset.fileId = file.id;
    card.querySelector('.file-icon').dataset.extension = file.extension || 'FIL';
    card.querySelector('.file-name').textContent = file.originalName;
    card.querySelector('.file-description').textContent = file.description;
    card.querySelector('.file-size').textContent = formatSize(file.size);
    card.querySelector('.file-remaining').textContent = formatRemaining(file.expiresAt);
    configureDownloadAction(card.querySelector('.download-button'), file);
    card.querySelector('.delete-button').addEventListener('click', () => deleteFile(file));
    fileList.append(card);
  }
}

async function loadFiles({ quiet = false } = {}) {
  if (!quiet) setStatus('Hämtar filer …');
  try {
    const response = await fetch('/api/files');
    if (!response.ok) throw new Error('Kunde inte hämta fillistan.');
    files = await response.json();
    renderFiles();
    setStatus('');
  } catch (error) {
    setStatus(error.message, true);
  }
}

async function deleteFile(file) {
  if (!window.confirm(`Radera ”${file.originalName}” direkt?`)) return;
  try {
    const response = await fetch(`/api/files/${encodeURIComponent(file.id)}`, { method: 'DELETE' });
    if (!response.ok && response.status !== 404) throw new Error('Filen kunde inte raderas.');
    files = files.filter((entry) => entry.id !== file.id);
    renderFiles();
    setStatus(`”${file.originalName}” har raderats.`);
  } catch (error) {
    setStatus(error.message, true);
  }
}

async function showPhoneDialog() {
  phoneDialog.showModal();
  qrPanel.innerHTML = '<p>Läser in lokal adress …</p>';
  try {
    const response = await fetch('/api/info');
    if (!response.ok) throw new Error('Kunde inte skapa QR-koden.');
    const info = await response.json();
    qrPanel.replaceChildren();
    const image = document.createElement('img');
    image.src = info.qrCode;
    image.alt = `QR-kod till ${info.lanUrl}`;
    const help = document.createElement('p');
    help.className = 'qr-help';
    help.textContent = 'Anslut telefonen till samma nätverk och skanna koden.';
    const link = document.createElement('a');
    link.href = info.lanUrl;
    link.textContent = info.lanUrl;
    qrPanel.append(image, help, link);
  } catch (error) {
    qrPanel.textContent = error.message;
  }
}

lifetime.addEventListener('input', () => {
  lifetimeLabel.textContent = formatLifetime(Number(lifetime.value));
});

uploadForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  submitUpload.disabled = true;
  submitUpload.textContent = 'Laddar upp …';
  try {
    const response = await fetch('/api/files', { method: 'POST', body: new FormData(uploadForm) });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || 'Filen kunde inte laddas upp.');
    uploadDialog.close();
    uploadForm.reset();
    lifetimeLabel.textContent = '8 timmar';
    await loadFiles({ quiet: true });
    setStatus(`”${result.originalName}” har laddats upp.`);
  } catch (error) {
    setStatus(error.message, true);
  } finally {
    submitUpload.disabled = false;
    submitUpload.textContent = 'Ladda upp';
  }
});

document.querySelector('#open-upload').addEventListener('click', () => uploadDialog.showModal());
document.querySelector('#open-phone').addEventListener('click', showPhoneDialog);
document.querySelector('#refresh-files').addEventListener('click', () => loadFiles());

document.querySelectorAll('[data-close-dialog]').forEach((button) => {
  button.addEventListener('click', () => button.closest('dialog').close());
});

document.querySelectorAll('dialog').forEach((dialog) => {
  dialog.addEventListener('click', (event) => {
    if (event.target === dialog) dialog.close();
  });
});

loadFiles();
setInterval(() => {
  let removedAny = false;
  files = files.filter((file) => {
    const active = Date.parse(file.expiresAt) > Date.now();
    removedAny ||= !active;
    return active;
  });
  if (removedAny) renderFiles();
  document.querySelectorAll('.file-card').forEach((card) => {
    const file = files.find((entry) => entry.id === card.dataset.fileId);
    if (file) card.querySelector('.file-remaining').textContent = formatRemaining(file.expiresAt);
  });
}, 30_000);
