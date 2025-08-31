const logContainer = document.getElementById('log-container');
const statusContainer = document.getElementById('status-container');
let pollingInterval;

// 启动服务器的函数
async function startServer() {
	try {
		const response = await fetch('/api/start');
		const text = await response.text();
		logContainer.textContent = text;
		// 启动服务器后，开始轮询日志
		startPolling();
	} catch (error) {
		logContainer.textContent = '无法启动服务器：' + error;
	}
}

// 停止服务器的函数
async function stopServer() {
	stopPolling(); // 停止日志刷新
	try {
		const response = await fetch('/api/stop');
		const text = await response.text();
		logContainer.textContent = text;
		refresh();
	} catch (error) {
		logContainer.textContent = '无法停止服务器：' + error;
	}
}

// 停止轮询日志的函数
function stopPolling() {
	clearInterval(pollingInterval);
	console.log("日志轮询已停止。");
}

// 开始轮询日志的函数
function startPolling() {
	if (pollingInterval) {
		clearInterval(pollingInterval);
	}
	pollingInterval = setInterval(refresh, 5000);
}

// 获取日志并更新页面的函数
async function refresh() {
	try {
		checkServerStatus();
		//Decide based on the status
		const response = await fetch('/api/checkstart');
        const text = await response.text();

		if (text.includes('running')) {
			const logResponse = await fetch('/api/log');
			const logText = await logResponse.text();

			logContainer.textContent = logText;
			logContainer.scrollTop = logContainer.scrollHeight;
			if (!pollingInterval) {
				startPolling();
			}
			return
		}
		
		// Server is not running, show a message and exit
		logContainer.textContent = "Server not running";
	} catch (error) {
		console.error("Failed to fetch logs or check status:", error);
		logContainer.textContent = "An error occurred while trying to connect to the server.";
	}
}

async function checkServerStatus() {
	try {
		const response = await fetch('/api/checkstart');
		const text = await response.text();
	
		if (text.includes('running')) {
			statusContainer.textContent = 'running';
			statusContainer.className = 'status-running';
			if (!pollingInterval) {
				startPolling();
			}
		} else {
			statusContainer.textContent = 'stopped';
			statusContainer.className = 'status-stopped';
			stopPolling();
			logContainer.textContent = "Server not running";
		}
	} catch (error) {
		statusContainer.textContent = '状态: 错误';
		statusContainer.className = 'status-stopped';
		console.error("检查状态失败:", error);
		stopPolling();
	}
}

async function sendCommand() {
	const commandInput = document.getElementById('command-input');
	const command = commandInput.value.trim(); // 使用 .trim() 移除前后空格

	if (command === '') {
		alert("请输入命令！");
		return;
	}

	try {
		const response = await fetch('/api/command', {
			method: 'POST',
			// 告诉后端我们发送的是纯文本
			headers: {
				'Content-Type': 'text/plain'
			},
			// 直接发送纯文本字符串作为请求体
			body: command
		});

		if (!response.ok) {
			const errorText = await response.text();
			throw new Error(`error: ${errorText}`);
		}

		const responseText = await response.text();
		console.log('response:', responseText);

		logContainer.textContent = responseText;
		stopPolling();
		commandInput.value = '';

	} catch (error) {
		console.error('failed to set:', error);
		alert(`command failed to send: ${error.message}`);
	}

	commandInput.value = '';
}

window.onload = function() {
	checkServerStatus();
}
 
