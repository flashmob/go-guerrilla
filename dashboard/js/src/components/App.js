import React, { Component } from 'react';
import {connect} from 'react-redux';
import LineChart from './LineChart';
import RankingTable from './RankingTable';

const styles = {
	container: {
		display: 'flex',
		width: '100%',
		flexDirection: 'column',
		justifyContent: 'space-between',
		padding: '64px 32px'
	},
	chartsContainer: {
		display: 'flex',
		flexDirection: 'column',
		justifyContent: 'space-between'
	},
	tableContainer: {
		display: 'flex',
		flexDirection: 'row',
		justifyContent: 'space-around'
	}
}

let WS_URL = `ws://${location.host}/ws`
if (process.env.NODE_ENV === 'development') {
	WS_URL = `ws://localhost:8080/ws`
}

// Maximum size of ranking tables
const RANKING_SIZE = 5;

const _computeRanking = mapping => {
	return Object.keys(mapping)
		.map(k => ({value: k, count: mapping[k]}))
		.sort((a, b) => b.count - a.count)
		.slice(0, RANKING_SIZE);
}

class App extends Component {
	constructor(props) {
		super(props);
		const ws = new WebSocket(WS_URL);
		ws.onerror = err => console.log(err);
		ws.onopen = event => console.log(event);
		ws.onclose = event => console.log(event);
		ws.onmessage = event => {
			const message = JSON.parse(event.data);
			props.dispatch(message);
		};

		this.state = {ws};
	}

	render() {
		const {ram, nClients, topDomain, topHelo, topIP} = this.props;
		return (
			<div style={styles.container}>
				<div style={styles.chartContainer}>
					<LineChart
						data={ram.get('data').toArray()}
						domain={[ram.get('min'), ram.get('max')]}
						format="bytes"
						title="Memory Usage" />
					<LineChart
						data={nClients.get('data').toArray()}
						domain={[nClients.get('min'), nClients.get('max')]}
						format="number"
						title="Number of Clients" />
				</div>
				<div style={styles.tableContainer}>
					<RankingTable
						rankType="Domain"
						ranking={_computeRanking(topDomain.toJS())} />
					<RankingTable
						rankType="HELO"
						ranking={_computeRanking(topHelo.toJS())} />
					<RankingTable
						rankType="IP"
						ranking={_computeRanking(topIP.toJS())} />
				</div>
			</div>
		);
	}
}

const mapStateToProps = state => ({
	ram: state.get('ram'),
	nClients: state.get('nClients'),
	topDomain: state.get('topDomain'),
	topHelo: state.get('topHelo'),
	topIP: state.get('topIP')
});

export default connect(mapStateToProps)(App);
