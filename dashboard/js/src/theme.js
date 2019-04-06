const { assign } = Object;

// Colors
const colors = [
	"#252525",
	"#525252",
	"#737373",
	"#969696",
	"#bdbdbd",
	"#d9d9d9",
	"#f0f0f0"
];

const charcoal = "#252525";

// Typography
const sansSerif = "'Gill Sans', 'Gill Sans MT', 'Ser­avek', 'Trebuchet MS', sans-serif";
const letterSpacing = "normal";
const fontSize = 12;

// Layout
const baseProps = {
	width: 450,
	height: 300,
	padding: {
		top: 20,
		bottom: 40,
		left: 80,
		right: 10
	},
	colorScale: colors
};

// Labels
const baseLabelStyles = {
	fontFamily: sansSerif,
	fontSize,
	letterSpacing,
	padding: 8,
	fill: charcoal,
	stroke: "transparent"
};

const centeredLabelStyles = assign({ textAnchor: "middle" }, baseLabelStyles);

// Strokes
const strokeLinecap = "round";
const strokeLinejoin = "round";

// Create the theme
const theme = {
	area: assign({
		style: {
			data: {
				fill: charcoal
			},
			labels: centeredLabelStyles
		}
	}, baseProps),
	axis: assign({
		style: {
			axis: {
				fill: "transparent",
				stroke: charcoal,
				strokeWidth: 1,
				strokeLinecap,
				strokeLinejoin
			},
			axisLabel: assign({}, centeredLabelStyles, {
				padding: 5
			}),
			grid: {
				fill: "transparent",
				stroke: "#f0f0f0"
			},
			ticks: {
				fill: "transparent",
				size: 1,
				stroke: "transparent"
			},
			tickLabels: baseLabelStyles
		}
	}, baseProps),
	bar: assign({
		style: {
			data: {
				fill: charcoal,
				padding: 10,
				stroke: "transparent",
				strokeWidth: 0,
				width: 8
			},
			labels: baseLabelStyles
		}
	}, baseProps),
	candlestick: assign({
		style: {
			data: {
				stroke: charcoal,
				strokeWidth: 1
			},
			labels: centeredLabelStyles
		},
		candleColors: {
			positive: "#ffffff",
			negative: charcoal
		}
	}, baseProps),
	chart: baseProps,
	errorbar: assign({
		style: {
			data: {
				fill: "transparent",
				stroke: charcoal,
				strokeWidth: 2
			},
			labels: centeredLabelStyles
		}
	}, baseProps),
	group: assign({
		colorScale: colors
	}, baseProps),
	line: assign({
		style: {
			data: {
				fill: "transparent",
				stroke: "#969696",
				strokeWidth: 2
			},
			labels: assign({}, baseLabelStyles, {
				textAnchor: "start"
			})
		}
	}, baseProps),
	pie: {
		style: {
			data: {
				padding: 10,
				stroke: "transparent",
				strokeWidth: 1
			},
			labels: assign({}, baseLabelStyles, {
				padding: 20
			})
		},
		colorScale: colors,
		width: 400,
		height: 400,
		padding: 50
	},
	scatter: assign({
		style: {
			data: {
				fill: charcoal,
				stroke: "transparent",
				strokeWidth: 0
			},
			labels: centeredLabelStyles
		}
	}, baseProps),
	stack: assign({
		colorScale: colors
	}, baseProps),
	tooltip: assign({
		style: {
			data: {
				fill: "transparent",
				stroke: "transparent",
				strokeWidth: 0
			},
			labels: centeredLabelStyles,
			flyout: {
				stroke: charcoal,
				strokeWidth: 1,
				fill: "#f0f0f0"
			}
		},
		flyoutProps: {
			cornerRadius: 10,
			pointerLength: 10
		}
	}, baseProps),
	voronoi: assign({
		style: {
			data: {
				fill: "transparent",
				stroke: "transparent",
				strokeWidth: 0
			},
			labels: centeredLabelStyles
		}
	}, baseProps)
};

export default theme;
