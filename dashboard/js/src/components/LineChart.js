import React, { PropTypes } from 'react';
import { VictoryAxis, VictoryChart, VictoryLine } from 'victory';
import { formatBytes, formatNumber } from './../util';
import moment from 'moment';
import theme from './../theme';

const _formatIndependentAxis = tick => moment(tick).format('HH:mm:ss');

const _formatDependentAxis = (tick, format) => (
	format === 'bytes' ?
		formatBytes(tick, 1) :
		formatNumber(tick, 1)
);

const LineChart = ({data, format}) => {
	return (
		<VictoryChart
			theme={theme}
			height={150}
			width={1000}>
			<VictoryAxis
				scale="time"
				tickCount={4}
				tickFormat={tick => _formatIndependentAxis(tick)}/>
			<VictoryAxis
				dependentAxis
				scale="linear"
				tickCount={2}
				tickFormat={tick => _formatDependentAxis(tick, format)} />
			<VictoryLine data={data} />
		</VictoryChart>
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
