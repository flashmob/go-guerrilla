import React, { Component } from 'react';
import {connect} from 'react-redux';
import {init, tick} from '../action-creators';
import LineChart from './LineChart';

const styles = {
	container: {
		backgroundSize: 'cover',
		display: 'flex',
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
		ws.onclose = event => console.log(event);
		ws.onmessage = ({data}) => props.dispatch(tick(data));

		this.state = {ws};
	}

	render() {
		return (
			<div style={styles.container}>
				<LineChart />
			</div>
		);
	}
}

export default connect()(App);
