<!DOCTYPE html>
<html>
<head>
	<link rel="icon" href="data:,">
	<title>VPC Conf Authentication</title>
	<meta charset="UTF-8">
	<link rel="stylesheet" href="/static/ver:{{.StaticAssetsVersion}}/css/cms-design/layout.css">
	<link rel="stylesheet" href="/static/ver:{{.StaticAssetsVersion}}/css/cms-design/core.css">
	<link rel="stylesheet" href="/static/ver:{{.StaticAssetsVersion}}/css/common.css">
	<script src="/static/ver:{{.StaticAssetsVersion}}/msal/msal-browser.min.js"></script>
</head>
<body class="ds-u-sans ds-u-font-size--base" style="background-color: #7E7E7E">
	<div class="ds-u-display--flex ds-u-justify-content--center ds-u-padding--1" style="height: 100vh;">
		<div style="background-color: #fff; margin: auto;">
			<div id="header" class="section-header-secondary-primary" style="border-radius: 0;">Authenticating...</div>
			<div class="section-body-bordered center" style="padding: 20px;width: 320px;height:60px;margin: 0;">
				<div id="message">Contacting server . . .</div>
				<div id="close" class="hidden ds-u-justify-content--center">
					<button type="button" id="closeButton" class="ds-c-button ds-c-button--primary ds-u-margin-top--1">Close Now</button>
				</div>
			</div>
		</div>
	</div>
</body>
<script type="module">
	import {User} from '/static/ver:{{.StaticAssetsVersion}}/view/user.js'

	const validate = async (claim) => {
		const header = document.getElementById("header");
		const message = document.getElementById("message");
		const close = document.getElementById("close");
		const closeButton = document.getElementById("closeButton");
		closeButton.addEventListener('click', (e) => {
			window.close();
		})
		try {
			const response = await fetch('/provision/oauth/validate', {
				method: 'POST',
				credentials: 'include',
				headers: {
					'Authorization': 'Bearer ' + claim.idToken,
					'X-Access': 'Bearer ' + claim.accessToken
				}
			});
			if (response.status == 200) {
				var seconds = 4
				header.innerHTML = 'Authentication Successful';
				message.innerHTML = 'This tab will auto-close in 5 seconds';
				const user = await response.json()
				User.setUser(user.name, user.isAdmin);

				setInterval(() => {
					if (seconds == 0) {
						window.close();
						return;
					}
					message.innerHTML = `This tab will auto-close in ${seconds} ${seconds == 1 ? 'second' : 'seconds'}`;
					seconds--;
				}, 1000)
			} else {
				throw "bad response status " + response.status
			}			
		} catch (err) {
			header.innerHTML = 'Authentication Failed';
			header.classList.add("error-background")
			message.innerText = 'Please close this tab and try again.';
			User.clearDetails();
			console.error("token validation error: " + err)
		}
		close.classList.remove('hidden');
	}

	const msalConfig = {
		auth: {
			clientId: '{{.ClientID}}',
			authority: '{{.Host}}/{{.TenantID}}',
			redirectUri: "{{.RedirectURL}}"
		},
			cache: {
			cacheLocation: "sessionStorage",
			storeAuthStateInCookie: false
		}
	};
	const msalInstance = new msal.PublicClientApplication(msalConfig);

	const request = { 
			scopes: ["user.read"]
	};

	msalInstance.handleRedirectPromise()
		.then(handleResponse)
		.catch(err => {
			message.innerHTML = err.toString();
		});

	function handleResponse(claim) {
		if (claim !== null) {
			validate(claim);
		} else {
			msalInstance.loginRedirect(request);
		}
	}
</script>
</html>
