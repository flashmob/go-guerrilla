import React, { PropTypes } from 'react';

const border = {
	borderColor: '#ddd',
	borderStyle: 'solid',
	borderWidth: 1
}

const styles = {
	table: {
		margin: '0 8px',
		borderCollapse: 'collapse',
		...border
	},
	row: {
		...border
	},
	odd: {
		backgroundColor: '#eee'
	},
	header: {
		backgroundColor: '#77D38F',
		padding: 8,
		fontWeight: 300,
		textAlign: 'left',
		...border
	},
	rank: {
		width: '15%'
	},
	rankType: {
		width: '60%'
	},
	count: {
		width: '25%'
	},
	cell: {
		padding: 4,
		...border
	}
}

const RankingTable = ({ranking, rankType}) => {
	return (
		<div>
			<table style={styles.table}>
				<caption>{`Top Clients by ${rankType}`}</caption>
				<tr style={styles.row}>
					<th style={{...styles.header, ...styles.rank}}>Rank</th>
					<th style={{...styles.header, ...styles.rankType}}>{rankType}</th>
					<th style={{...styles.header, ...styles.count}}># Clients</th>
				</tr>
				{
					ranking.map((record, i) => (
						<tr style={Object.assign({},
							styles.row,
							i % 2 === 0 && styles.odd
						)} key={record.value}>
							<td style={styles.cell}>{i + 1}</td>
							<td style={styles.cell}>{record.value}</td>
							<td style={styles.cell}>{record.count}</td>
						</tr>
					))
				}
			</table>
		</div>
	)
};

RankingTable.propTypes = {
	ranking: PropTypes.arrayOf(PropTypes.shape({
		value: PropTypes.string,
		count: PropTypes.number
	})),
	rankType: PropTypes.oneOf(['IP', 'Domain', 'HELO'])
};

export default RankingTable;
