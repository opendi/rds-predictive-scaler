interface Snapshot {
    timestamp: string;
    max_cpu_utilization: number;
    num_readers: number;
    cluster_utilization: number;
    predicted_value: boolean;
    future_value: boolean;
    cluster_name: string;
}

export default Snapshot;