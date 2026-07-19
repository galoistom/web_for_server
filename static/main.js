const logContainer = document.getElementById('log-container')
const statusContainer = document.getElementById('status-container')
const commandInput = document.getElementById('command-input')
const sendBtn = document.getElementById('send-btn')
const themeToggle = document.getElementById('theme-toggle')

let pollingInterval = null

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

async function checkServerStatus() {
	try {
		const { text } = await apiGet('/api/checkstart')
		const running = text.includes('running')
		statusContainer.textContent = running ? 'running' : 'stopped'
		statusContainer.className = running ? 'status-running' : 'status-stopped'
		setCommandEnabled(running)
		return running
	} catch {
		statusContainer.textContent = 'error'
		statusContainer.className = 'status-stopped'
		setCommandEnabled(false)
		return false
	}
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
        logContainer.textContent = '无法停止服务器：' + err
    }
    await sleep(1000)
    await checkServerStatus()
}

async function sendCommand() {
	const command = commandInput.value.trim()
	if (!command) return

	try {
		const res = await apiPost('/api/command', command)
		logContainer.textContent = res.ok ? res.text : '错误：' + res.text
		commandInput.value = ''
	} catch (err) {
		logContainer.textContent = '命令发送失败：' + err
	}
}

commandInput.addEventListener('keydown', (e) => {
	if (e.key === 'Enter') sendCommand()
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

refresh()
