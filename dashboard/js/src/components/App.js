import React, { Component } from 'react';
import {connect} from 'react-redux';
import {init, tick} from '../action-creators';
import LineChart from './LineChart';

const styles = {
	container: {
		backgroundSize: 'cover',
		display: 'flex',
		padding: 64,
		flexDirection: 'column'
	},
	chartContainer: {

	}
}

const WS_URL = 'ws://localhost:8080/ws';

class App extends Component {
	constructor(props) {
		super();
		const ws = new WebSocket(WS_URL);
		ws.onerror = err => console.log(err);
		ws.onopen = event => console.log(event);
		ws.onclose = event => console.log(event);
		ws.onmessage = event => {
			const data = JSON.parse(event.data);
			props.dispatch(tick(data));
		};

		this.state = {ws};
	}

	render() {
		return (
			<div style={styles.container}>
				<LineChart data={this.props.ram} />
			</div>
		);
	}
}

const mapStateToProps = state => ({
	ram: state.get('ram').toArray(),
	nClients: state.get('nClients').toArray()
});

export default connect(mapStateToProps)(App);
