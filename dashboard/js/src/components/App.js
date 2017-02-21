import React, { Component } from 'react';
import {connect} from 'react-redux';
import {init, tick} from '../action-creators';
import ActionTypes from '../action-types';
import LineChart from './LineChart';

const styles = {
	container: {
		backgroundSize: 'cover',
		display: 'flex',
		padding: 32,
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
			const message = JSON.parse(event.data);
			console.log(message);
			props.dispatch(message);
		};

		this.state = {ws};
	}

	render() {
		const {ram, nClients} = this.props;
		return (
			<div style={styles.container}>
				<LineChart
					data={ram.get('data').toArray()}
					domain={[ram.get('min'), ram.get('max')]}
					format="bytes" />
				<LineChart
					data={nClients.get('data').toArray()}
					domain={[nClients.get('min'), nClients.get('max')]}
					format="number" />
			</div>
		);
	}
}

const mapStateToProps = state => ({
	ram: state.get('ram'),
	nClients: state.get('nClients')
});

export default connect(mapStateToProps)(App);
