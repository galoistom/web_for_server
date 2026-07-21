const logContainer = document.getElementById('log-container')
const statusContainer = document.getElementById('status-container')
const commandInput = document.getElementById('command-input')
const sendBtn = document.getElementById('send-btn')
const themeToggle = document.getElementById('theme-toggle')
const fileToggle = document.getElementById('file-toggle')
const filePanel = document.getElementById('file-panel')
const fileOverlay = document.getElementById('file-overlay')
const filePanelClose = document.getElementById('file-panel-close')
const fileList = document.getElementById('file-list')
const fileEditor = document.getElementById('file-editor')
const fileEditorContent = document.getElementById('file-editor-content')
const fileEditorInfo = document.getElementById('file-editor-info')
const fileSaveBtn = document.getElementById('file-save-btn')
const fileSaveMsg = document.getElementById('file-save-msg')

let pollingInterval = null
let currentFileId = null

function setCommandEnabled(enabled) {
	commandInput.disabled = !enabled
	sendBtn.disabled = !enabled
}

function setPolling(shouldPoll) {
	if (shouldPoll && !pollingInterval) {
		pollingInterval = setInterval(refresh, 3000)
	} else if (!shouldPoll && pollingInterval) {
		clearInterval(pollingInterval)
		pollingInterval = null
	}
}

async function apiGet(path) {
	const res = await fetch(path)
	return { ok: res.ok, text: await res.text() }
}

async function apiPost(path, body) {
	const res = await fetch(path, {
		method: 'POST',
		headers: { 'Content-Type': 'text/plain' },
		body: body,
	})
	return { ok: res.ok, text: await res.text() }
}

async function apiPostJSON(path, body) {
	const res = await fetch(path, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body),
	})
	return { ok: res.ok, text: await res.text() }
}

async function checkServerStatus() {
	try {
		const { text } = await apiGet('/api/checkstart')
		const running = text.includes('running')
		statusContainer.textContent = running ? 'running' : 'stopped'
		statusContainer.className = running ? 'status-running' : 'status-stopped'
		setCommandEnabled(running)
		updateEditorLock(running, fileEditor.dataset.preview === 'true')
		return running
	} catch {
		statusContainer.textContent = 'error'
		statusContainer.className = 'status-stopped'
		setCommandEnabled(false)
		updateEditorLock(false, fileEditor.dataset.preview === 'true')
		return false
	}
}

function updateEditorLock(running, preview) {
	const locked = running || preview
	fileEditorContent.disabled = locked
	fileSaveBtn.disabled = locked
	fileSaveMsg.textContent = preview ? '此文件为只读预览' : running ? '服务器正在运行，无法编辑文件' : ''
}

async function fetchLog() {
	try {
		const { text } = await apiGet('/api/log')
		logContainer.textContent = text
		requestAnimationFrame(() => {
			logContainer.scrollTop = logContainer.scrollHeight
		})
	} catch {
		logContainer.textContent = 'unable to fetch log'
	}
}

async function refresh() {
	const running = await checkServerStatus()
	setPolling(running)
	if (running) {
		await fetchLog()
	} else {
		logContainer.innerHTML = '<div class="log-placeholder">server not running</div>'
	}
}

async function startServer() {
	try {
		const { ok, text } = await apiGet('/api/start')
		if (ok) {
			statusContainer.textContent = 'running'
			statusContainer.className = 'status-running'
			setCommandEnabled(true)
			setPolling(true)
			await fetchLog()
		} else {
			logContainer.textContent = text
		}
	} catch (err) {
	    logContainer.textContent = 'unable to start server: ' + err
	}
}

function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

async function stopServer() {
    setPolling(false)
    try {
        const res = await apiGet('/api/stop')
        logContainer.textContent = res.text
    } catch (err) {
        logContainer.textContent = 'failed to stop server: ' + err
    }
    await sleep(1000)
    await checkServerStatus()
}

async function sendCommand() {
	const command = commandInput.value.replace(/\r/g, '').trim()
	if (!command) return

	try {
		const res = await apiPost('/api/commands', command)
		logContainer.textContent = res.ok ? res.text : 'error: ' + res.text
		commandInput.value = ''
		commandInput.rows = 1
	} catch (err) {
		logContainer.textContent = '命令发送失败：' + err
		commandInput.rows = 1
	}
}

commandInput.addEventListener('keydown', (e) => {
	if (e.key === 'Enter' && !e.shiftKey) {
		e.preventDefault()
		sendCommand()
	}
})

commandInput.addEventListener('input', () => {
	commandInput.rows = commandInput.value.split('\n').length
})

// dark mode
const saved = localStorage.getItem('theme')
if (saved === 'dark') {
	document.documentElement.setAttribute('data-theme', 'dark')
	themeToggle.textContent = '☀️'
}

themeToggle.addEventListener('click', () => {
	const isDark = document.documentElement.getAttribute('data-theme') === 'dark'
	if (isDark) {
		document.documentElement.removeAttribute('data-theme')
		localStorage.setItem('theme', 'light')
		themeToggle.textContent = '🌙'
	} else {
		document.documentElement.setAttribute('data-theme', 'dark')
		localStorage.setItem('theme', 'dark')
		themeToggle.textContent = '☀️'
	}
})

// file panel
function openFilePanel() {
	filePanel.classList.remove('hidden')
	fileOverlay.classList.remove('hidden')
	loadFileList()
}

function closeFilePanel() {
	filePanel.classList.add('hidden')
	fileOverlay.classList.add('hidden')
	currentFileId = null
	fileEditor.classList.add('hidden')
}

fileToggle.addEventListener('click', openFilePanel)
filePanelClose.addEventListener('click', closeFilePanel)
fileOverlay.addEventListener('click', closeFilePanel)

async function loadFileList() {
	fileList.innerHTML = '<div class="log-placeholder" style="padding:12px 20px;text-align:left">加载中...</div>'
	try {
		const res = await fetch('/api/file')
		const files = await res.json()
		fileList.innerHTML = ''
		for (const f of files) {
			const item = document.createElement('div')
			item.className = 'file-list-item'
			item.dataset.id = f.id
			const icon = f.type === 'json' ? '📋' : '📄'
			item.innerHTML = `
				<span class="file-icon">${icon}</span>
				<span class="file-name">${f.name}</span>
				${f.preview ? '<span class="file-lock">👁️</span>' : ''}
			`
			item.addEventListener('click', () => editFile(f.id))
			fileList.appendChild(item)
		}
		if (files.length === 0) {
			fileList.innerHTML = '<div class="log-placeholder" style="padding:12px 20px;text-align:left">没有可编辑的文件</div>'
		}
	} catch {
		fileList.innerHTML = '<div class="log-placeholder" style="padding:12px 20px;text-align:left">加载失败</div>'
	}
}

async function editFile(id) {
	document.querySelectorAll('.file-list-item').forEach(el => el.classList.remove('active'))
	const item = document.querySelector(`.file-list-item[data-id="${id}"]`)
	if (item) item.classList.add('active')

	currentFileId = id

	try {
		const res = await fetch(`/api/file/edit?id=${encodeURIComponent(id)}`)
		const data = await res.json()
		fileEditorInfo.textContent = `📁 ${data.name}  (${data.path || data.id})`
		fileEditorContent.value = data.content
		fileEditor.classList.remove('hidden')

		fileEditor.dataset.preview = data.preview ? 'true' : 'false'
		updateEditorLock(statusContainer.textContent === 'running', data.preview)
		fileSaveMsg.textContent = ''
	} catch {
		fileSaveMsg.textContent = '加载失败'
	}
}

async function saveFile() {
	if (!currentFileId) return
	fileSaveMsg.textContent = '保存中...'
	fileSaveBtn.disabled = true
	try {
		const res = await apiPostJSON('/api/file/save', {
			id: currentFileId,
			content: fileEditorContent.value,
		})
		fileSaveMsg.textContent = res.ok ? '✅ 已保存' : '❌ ' + res.text
	} catch {
		fileSaveMsg.textContent = '❌ 保存失败'
	}
	fileSaveBtn.disabled = false
	setTimeout(() => {
		if (fileSaveMsg.textContent === '✅ 已保存') fileSaveMsg.textContent = ''
	}, 3000)
}

fileSaveBtn.addEventListener('click', saveFile)

refresh()
