export class User {
	constructor() {
		// this class listens for its own storage event fired in setUser() to
		// enable cross-tab communication. the user login sequence is performed
		// in a new tab and this allows the parent tab to automatically update
		window.addEventListener('storage', (e) => {
			if (e.key != 'user-set' || e.newValue != "true") return;
			User._fireUserEvent();
		});
	}

	static clearDetails = () => {
		localStorage.removeItem('name');
		localStorage.removeItem('isAdmin');
		localStorage.setItem('user-set', "true");
		localStorage.removeItem('user-set');
	}
	
	static _fireUserEvent = () => {
		window.dispatchEvent(new CustomEvent('user-ready'));
	}

	static isAdmin = () => {
		return localStorage.getItem("isAdmin") == "true";
	}

	static name = () => {
		return localStorage.getItem('name');
	}

	static setUser = (name, isAdmin) => {
		localStorage.setItem('name', name);
		localStorage.setItem('isAdmin', isAdmin);
		localStorage.setItem('user-set', "true");
		localStorage.removeItem('user-set');
	}
}
