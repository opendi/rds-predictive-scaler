interface ClusterStatus {
    identifier: string;
    timestamp: Date;
    average_cpu_utilization: number;
    current_active_readers: number;
    optimal_size: number;
    instance_status: InstanceStatus[];
}