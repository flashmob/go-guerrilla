package backends

type StreamBackendGateway struct {
	BackendGateway
	config *StreamBackendConfig
}

type StreamBackendConfig struct {
	StreamSaveProcess string `json:"stream_save_process,omitempty"`
}

/*

{"c":6,"tp":0,"hp":-1,"a":true,"p":[{"i":"1","s":87,"h":["ZnIOFj5TN9mANjBj2+C43Q"],"t":"multipart/mixed; boundary=\"D7F------------D7FD5A0B8AB9C65CCDBFA872\"","c":"","e":"","d":"","cb":"D7F------------D7FD5A0B8AB9C65CCDBFA872"},{"i":"1.1","s":180,"h":["WUPYSAuGmo0X2M0dlBPQPQ","60GpUAQjInSlsshIhg3lbg"],"t":"text/plain; charset=\"us-ascii\"","c":"us-ascii","e":"7bit","d":"","cb":"D7F------------D7FD5A0B8AB9C65CCDBFA872"},{"i":"1.2","s":878,"h":["8A9m4qGsTU4wQB1wAgBEVw","jn8wKuYo7bK2+S2Bd6ySVA"],"t":"message/rfc822","c":"","e":"7bit","d":"inline","cb":"D7F------------D7FD5A0B8AB9C65CCDBFA872"},{"i":"1.2.1","s":87,"h":["nLQXv1n/XZgen8SZmcoYnw"],"t":"multipart/mixed; boundary=\"DC8------------DC8638F443D87A7F0726DEF7\"","c":"","e":"","d":"","cb":"DC8------------DC8638F443D87A7F0726DEF7"},{"i":"1.2.1.1","s":469,"h":["XBczIVrikBfu+DLiyFuClA","0OjJJc8xNb2BlqU+1MaSCA"],"t":"text/plain; charset=\"us-ascii\"","c":"us-ascii","e":"7bit","d":"","cb":"DC8------------DC8638F443D87A7F0726DEF7"},{"i":"1.2.1.2","s":633,"h":["TWNp+1kio1xxZZBFMzZ2GA","/oA/Nr7g2e6AoWdmm52v/g"],"t":"image/gif; name=\"map_of_Argentina.gif\"","c":"","e":"base64","d":"attachment; filename=\"map_of_Argentina.gif\"","cb":"DC8------------DC8638F443D87A7F0726DEF7"}]}

*/
