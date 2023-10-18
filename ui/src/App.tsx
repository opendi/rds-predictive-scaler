import {useEffect, useState} from 'react';
import useWebSocket from 'react-use-websocket';
import Snapshot from './types/Snapshot';
import Broadcast from "./types/Broadcast";
import {Box, Chip, Container, createTheme, CssBaseline, Paper, ThemeProvider, Typography} from "@mui/material";
import Loading from "./components/Loading";
import CurrentLoad from "./components/CurrentLoad";
import CurrentSize from "./components/CurrentSize";
import GraphUtilization from "./components/GraphUtilization";
import GraphClusterSize from "./components/GraphClusterSize";
import ClusterMap, {InstanceStatus} from "./components/ClusterMap.tsx";
import {Config} from "./types/Config.ts";
import {MonitorHeart} from "@mui/icons-material";

const theme = createTheme({
    palette: {
        mode: 'dark', // Set the theme to dark mode
    },
    typography: {
        fontFamily: 'Lato, sans-serif',
    }
});

function App() {
    const socketUrl =
        'ws://' +
        (process.env.NODE_ENV === 'development'
            ? 'localhost:8041/'
            : 'localhost:8001/api/v1/namespaces/kube-system/services/http:rds-predictive-scaler:http/proxy/') +
        'ws';

    const { lastJsonMessage, readyState } = useWebSocket(socketUrl, {
        onOpen: () => {
            console.log('WebSocket connection established');
        },
        shouldReconnect: () => true,
    });

    const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
    const [clusterStatus, setClusterStatus] = useState<InstanceStatus[]>([]);
    const [currentSnapshot, setCurrentSnapshot] = useState<Snapshot | null>(null);
    const [currentPrediction, setCurrentPrediction] = useState<Snapshot | null>(null);
    const [scalerConfig, setScalerConfig] = useState<Config | null>(null);

    useEffect(() => {
        if (lastJsonMessage == null) return;
        const broadcast = lastJsonMessage as any as Broadcast;
        switch (broadcast.type) {
            case 'config':
                setScalerConfig(broadcast.data);
                break;
            case 'clusterStatus':
                setClusterStatus(broadcast.data);
                break;
            case 'snapshot':
                setSnapshots((snapshots) => [...snapshots.slice(1), broadcast.data]);
                setCurrentSnapshot(broadcast.data);
                break;
            case 'prediction':
                setCurrentPrediction(broadcast.data);
                break;
            case 'snapshots':
                setSnapshots(broadcast.data);
                break;

        }
    }, [lastJsonMessage]);

    const aggregatedData = groupDataByTime(snapshots, 1 * 60 * 1000, 5 * 1000);

    return (
        <ThemeProvider theme={theme}>
            <CssBaseline/>
            <Container>
                {readyState === 1 && currentSnapshot ? (
                    <Paper elevation={3} sx={{padding: 2, marginBottom: 3}}>
                        <Box sx={{width: '100%'}}>
                            <Typography variant="h4" gutterBottom>
                                <MonitorHeart fontSize={"large"}/> Cluster Status {currentSnapshot &&
                                <Chip label={currentSnapshot.cluster_name}/>}
                                {currentSnapshot && (
                                    <Box sx={{marginTop: '10px', display: 'flex'}}>
                                        <CurrentLoad currentSnapshot={currentSnapshot}
                                                     currentPrediction={currentPrediction}/>
                                        <CurrentSize currentSnapshot={currentSnapshot}
                                                     currentPrediction={currentPrediction}/>
                                    </Box>
                                )}
                            </Typography>
                        </Box>
                        <Box sx={{width: '100%'}}>
                            <GraphUtilization data={aggregatedData} targetCpuUtilization={scalerConfig?.TargetCpuUtil}/>
                            <GraphClusterSize data={aggregatedData}/>
                        </Box>
                        <Box sx={{width: '100%'}}>
                            <ClusterMap clusterStatus={clusterStatus}/>
                        </Box>
                    </Paper>
                ) : <Loading readyState={readyState} />}
            </Container>
        </ThemeProvider>
    );
}

function groupDataByTime(data: Snapshot[], timeInterval: number, mostRecentTimeInterval: number): Snapshot[] {
    const groupedData: Snapshot[] = [];
    let currentGroup: Snapshot | null = null;
    let numReadersSum = 0;
    let maxCpuUtilizationSum = 0;
    let count = 0;

    let mostRecentTimestamp = 0;
    for (let i = data.length - 1; i >= 0; i--) {
        if (!data[i].future_value) {
            mostRecentTimestamp = new Date(data[i].timestamp).getTime();
            break;
        }
    }

    data.forEach((snapshot) => {
        const timestamp = new Date(snapshot.timestamp).getTime();
        // Determine the appropriate time interval based on the conditions
        const interval = (!snapshot.future_value && mostRecentTimestamp - timestamp <= 15 * 60 * 1000)
            ? mostRecentTimeInterval
            : timeInterval;

        if (!currentGroup) {
            currentGroup = {
                ...snapshot,
                timestamp: snapshot.timestamp,
                num_readers: 0,
            } as Snapshot;
            numReadersSum = snapshot.num_readers;
            maxCpuUtilizationSum = snapshot.max_cpu_utilization;
            count = 1;
        } else if (timestamp - new Date(currentGroup.timestamp).getTime() >= interval) {
            (currentGroup as Snapshot).num_readers = numReadersSum / count;
            groupedData.push(currentGroup);
            currentGroup = {...snapshot, timestamp: snapshot.timestamp, num_readers: 0};
            numReadersSum = snapshot.num_readers;
            maxCpuUtilizationSum = snapshot.max_cpu_utilization;
            count = 1;
        } else {
            numReadersSum += snapshot.num_readers;
            maxCpuUtilizationSum += snapshot.max_cpu_utilization;
            count++;
        }
    });

    if (currentGroup) {
        (currentGroup as Snapshot).num_readers = numReadersSum / count;
        (currentGroup as Snapshot).max_cpu_utilization = maxCpuUtilizationSum / count;
        groupedData.push(currentGroup);
    }

    return groupedData;
}

export default App;
