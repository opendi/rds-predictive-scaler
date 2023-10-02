import React, {useEffect, useState} from 'react';
import useWebSocket from 'react-use-websocket';
import Snapshot from './types/Snapshot';
import Broadcast from "./types/Broadcast";
import {
    Box,
    Chip,
    Container,
    createTheme,
    CssBaseline,
    IconButton,
    Paper,
    ThemeProvider,
    Typography
} from "@mui/material";
import Loading from "./components/Loading";
import CurrentLoad from "./components/CurrentLoad";
import CurrentSize from "./components/CurrentSize";
import GraphUtilization from "./components/GraphUtilization";
import GraphClusterSize from "./components/GraphClusterSize";
import ScaleStatus from "./types/ScaleStatus";
import CooldownVisualizer from "./components/CooldownVisualizer";
import SettingsModal from "./components/SettingsModal";
import SettingsIcon from '@mui/icons-material/Settings';

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

    const {lastJsonMessage} = useWebSocket(socketUrl, {
        onOpen: () => console.log('websocket connection established'),
        shouldReconnect: () => true,
    });

    const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
    const [currentSnapshot, setCurrentSnapshot] = useState<Snapshot | null>(null);
    const [predictions, setPredictions] = useState<Snapshot[]>([]);
    const [currentPrediction, setCurrentPrediction] = useState<Snapshot | null>(null);
    const [scaleOutStatus, setScaleOutStatus] = useState<ScaleStatus | null>(null);
    const [scaleInStatus, setScaleInStatus] = useState<ScaleStatus | null>(null);
    const [isSettingsModalOpen, setSettingsModalOpen] = useState(false);

    useEffect(() => {
        if (lastJsonMessage == null) return;
        const broadcast = lastJsonMessage as any as Broadcast;
        switch (broadcast.type) {
            case 'scaleOutStatus':
                const scaleOutStatus = broadcast.data as ScaleStatus;
                setScaleOutStatus(scaleOutStatus);
                break;
            case 'scaleInStatus':
                const scaleInStatus = broadcast.data as ScaleStatus;
                setScaleOutStatus(scaleInStatus);
                break;
            case 'snapshot':
                const snapshot = broadcast.data as Snapshot;
                snapshot.num_readers += 1;
                snapshot.cluster_utilization = snapshot.max_cpu_utilization * snapshot.num_readers;
                setSnapshots((snapshots) => [...snapshots.slice(1), snapshot]);
                setCurrentSnapshot(snapshot);
                break;
            case 'prediction':
                const prediction = broadcast.data as Snapshot;
                prediction.num_readers += 1;
                prediction.cluster_utilization = prediction.max_cpu_utilization * prediction.num_readers;
                setPredictions((predictions) => [...predictions.slice(1), prediction]);
                setCurrentPrediction(prediction);
                break;
            case 'snapshots':
                setSnapshots((broadcast.data as Snapshot[]).map((snapshot: Snapshot) => {
                    snapshot.num_readers += 1;
                    snapshot.cluster_utilization = snapshot.max_cpu_utilization * snapshot.num_readers;
                    return snapshot;
                }));
                break;

        }
    }, [lastJsonMessage]);

    const aggregatedData = groupDataByTime(snapshots, 5 * 60 * 1000, 10 * 1000);
    const openSettingsModal = () => {
        setSettingsModalOpen(true);
    };

    const closeSettingsModal = () => {
        setSettingsModalOpen(false);
    };

    return (
        <ThemeProvider theme={theme}>
            <CssBaseline/>
            <Container>
                {currentSnapshot ? (
                    <Paper elevation={3} sx={{padding: 2, marginBottom: 3}}>
                        <Box sx={{width: '100%'}}>
                            <Typography variant="h4" gutterBottom>
                                Cluster Status {currentSnapshot && <Chip label={currentSnapshot.cluster_name}/>}
                                <IconButton onClick={openSettingsModal}><SettingsIcon/></IconButton>
                                {currentSnapshot && (
                                    <Box sx={{marginTop: '10px', display: 'flex'}}>
                                        <CurrentLoad currentSnapshot={currentSnapshot}
                                                     currentPrediction={currentPrediction}/>
                                        <CurrentSize currentSnapshot={currentSnapshot}
                                                     currentPrediction={currentPrediction}/>
                                        <CooldownVisualizer scaleInStatus={scaleInStatus}
                                                            scaleOutStatus={scaleOutStatus}/>
                                    </Box>
                                )}
                            </Typography>
                        </Box>
                        <GraphUtilization data={aggregatedData}/>
                        <GraphClusterSize data={aggregatedData}/>
                    </Paper>
                ) : <Loading/>}
                <SettingsModal open={isSettingsModalOpen} onClose={closeSettingsModal} onSave={() => {
                }}/>
            </Container>
        </ThemeProvider>
    );
}

function groupDataByTime(data: Snapshot[], timeInterval: number, mostRecentTimeInterval: number): Snapshot[] {
    const groupedData: Snapshot[] = [];
    let currentGroup: Snapshot | null = null;
    let numReadersSum = 0;
    let clusterUtilizationSum = 0;
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
                cluster_utilization: 0
            } as Snapshot;
            numReadersSum = snapshot.num_readers;
            clusterUtilizationSum = snapshot.cluster_utilization;
            maxCpuUtilizationSum = snapshot.max_cpu_utilization;
            count = 1;
        } else if (timestamp - new Date(currentGroup.timestamp).getTime() >= interval) {
            (currentGroup as Snapshot).num_readers = numReadersSum / count;
            (currentGroup as Snapshot).cluster_utilization = clusterUtilizationSum / count;
            groupedData.push(currentGroup);
            currentGroup = {...snapshot, timestamp: snapshot.timestamp, num_readers: 0, cluster_utilization: 0};
            numReadersSum = snapshot.num_readers;
            clusterUtilizationSum = snapshot.cluster_utilization;
            maxCpuUtilizationSum = snapshot.max_cpu_utilization;
            count = 1;
        } else {
            numReadersSum += snapshot.num_readers;
            clusterUtilizationSum += snapshot.cluster_utilization;
            maxCpuUtilizationSum += snapshot.max_cpu_utilization;
            count++;
        }
    });

    if (currentGroup) {
        (currentGroup as Snapshot).num_readers = numReadersSum / count;
        (currentGroup as Snapshot).cluster_utilization = clusterUtilizationSum / count;
        (currentGroup as Snapshot).max_cpu_utilization = maxCpuUtilizationSum / count;
        groupedData.push(currentGroup);
    }

    return groupedData;
}

export default App;
