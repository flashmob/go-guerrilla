import ActionTypes from './action-types';

export const tick = payload => ({
	type: ActionTypes.TICK,
	payload
});

export const init = payload => ({
	type: ActionTypes.INIT,
	payload
});
