import React, { PropTypes } from 'react';
import { VictoryAxis, VictoryChart, VictoryLine } from 'victory';
import { formatBytes, formatNumber } from './../util';
import moment from 'moment';
import simplify from 'simplify-js';
import theme from './../theme';

const _formatIndependentAxis = tick => {
	return moment(tick).format('HH:mm:ss');
};

const _formatDependentAxis = (tick, format) => (
	format === 'bytes' ?
		formatBytes(tick, 1) :
		formatNumber(tick, 1)
);

// Uses simplifyJS to simplify the data from the backend (there can be up to
// 8000 points so this step is necessary). Because of the format expectations
// of simplifyJS, we need to convert x's to ints and back to moments.
const _simplify = data => {
	if (data.length === 0) return [];
	return simplify(
		data.map(d => ({x: moment(d.x).valueOf(), y: d.y}))
	).map(d => ({x: moment(d.x), y: d.y}));
}

const styles = {
	title: {
		fontSize: 22,
		fontWeight: 300,
		textAlign: 'center',
		margin: 0
	}
};

const LineChart = ({data, format, title}) => {
	return (
		<div>
			<h1 style={styles.title}>{title}</h1>
			<VictoryChart
				theme={theme}
				height={200}
				width={1500}>
				<VictoryAxis
					scale="time"
					tickCount={4}
					tickFormat={tick => _formatIndependentAxis(tick)}/>
				<VictoryAxis
					dependentAxis
					scale="linear"
					tickCount={3}
					tickFormat={tick => _formatDependentAxis(tick, format)} />
				<VictoryLine data={_simplify(data)} />
			</VictoryChart>
		</div>
	);
};

LineChart.propTypes = {
	data: PropTypes.arrayOf(PropTypes.shape({
		x: PropTypes.instanceOf(moment),
		y: PropTypes.number
	})),
	format: PropTypes.oneOf(['bytes', 'number'])
};

LineChart.defaultProps = {
	data: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map(n => ({
		x: moment().add(n, 'minutes'),
		y: n * 100 * Math.random()
	})),
	format: 'number'
};

export default LineChart;
