import React, { PropTypes } from 'react';
import { VictoryChart, VictoryLine } from 'victory';
import Moment from 'moment';

const LineChart = ({data}) => {
	return (
		<VictoryChart
			height={200}
			width={800}
			scale={{x: "time", y: "linear"}}>
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
