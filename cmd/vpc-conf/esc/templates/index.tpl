<!DOCTYPE html>
<html>
<head>
	<link rel="icon" href="data:,">
	<title>VPC Conf</title>
	<meta charset="UTF-8">
	<link rel="stylesheet" href="/static/ver:{{.}}/css/cms-design/layout.css">
	<link rel="stylesheet" href="/static/ver:{{.}}/css/cms-design/core.css">
	<link rel="stylesheet" href="/static/ver:{{.}}/css/navigation.css">
	<link rel="stylesheet" href="/static/ver:{{.}}/css/tab-ui.css">
	<link rel="stylesheet" href="/static/ver:{{.}}/css/common.css">
	<script type="module" src="/static/ver:{{.}}/router.js"></script>
	<script type="module" src="/static/ver:{{.}}/navigation.js"></script>
	<script type="module" src="/static/ver:{{.}}/view/components/shared/breadcrumb.js"></script>
	<script type="module" src="/static/ver:{{.}}/view/components/shared/growl.js"></script>
</head>
<body class="ds-u-sans ds-u-font-size--base">
<growl-component></growl-component>
<vpc-conf-navigation></vpc-conf-navigation>
<breadcrumb-trail root='{"name": "Dashboard", "link": "/provision"}'></breadcrumb-trail>
<div id="mainContent"></div>
</body>
</html>
