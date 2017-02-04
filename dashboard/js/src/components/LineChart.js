import React, { PropTypes } from 'react';
import { VictoryAxis, VictoryChart, VictoryLine } from 'victory';
import Moment from 'moment';

const LineChart = ({data}) => {
	return (
		<VictoryChart
			height={200}
			width={800}>
			<VictoryAxis // 2017-02-04T10:52:20.765730186-08:00
				scale="time"
				tickCount={4}
				tickFormat={tick => Moment(tick).format('HH:mm:ss')}/>
			<VictoryLine data={data} />
		</VictoryChart>
	);
};

LineChart.propTypes = {
	data: PropTypes.arrayOf(PropTypes.shape({
		x: PropTypes.instanceOf(Moment),
		y: PropTypes.number
	}))
};

LineChart.defaultProps = {
	data: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map(n => ({
		x: Moment().add(n, 'minutes'),
		y: n * 100 * Math.random()
	}))
};

export default LineChart;
