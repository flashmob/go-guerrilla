import ActionTypes from './action-types';
import Moment from 'moment';

const TIME_FORMAT = 'YYYY-MM-DDTHH:mm:ss.SSSSSSSSSZ';

export const tick = ({ram, n_clients}) => ({
	type: ActionTypes.TICK,
	payload: {
		ram: {
			x: Moment(ram.x, TIME_FORMAT),
			y: ram.y
		},
		n_clients: {
			x: Moment(n_clients.x, TIME_FORMAT),
			y: n_clients.y
		}
	}
});

export const init = payload => ({
	type: ActionTypes.INIT,
	payload
});
