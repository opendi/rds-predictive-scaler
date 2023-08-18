import React, {useEffect, useState} from 'react';
import {Bar, CartesianGrid, Cell, ComposedChart, Line, ResponsiveContainer, Tooltip, XAxis, YAxis} from 'recharts';
import {
    Backdrop,
    Box, Chip,
    CircularProgress,
    Container,
    createTheme,
    CssBaseline,
    LinearProgress,
    Paper,
    Theme,
    ThemeProvider,
    Typography
} from '@mui/material';

import '@fontsource/lato/300.css';
import '@fontsource/lato/400.css';
import '@fontsource/lato/700.css';

interface Snapshot {
    timestamp: string;
    max_cpu_utilization: number;
    num_readers: number;
    cluster_utilization: number;
    predicted_value: boolean;
    future_value: boolean;
    cluster_name: string;
}

const theme = createTheme({
    palette: {
        mode: 'dark', // Set the theme to dark mode
    },
    typography: {
        fontFamily: 'Lato, sans-serif',
    }
});

async function fetchData(start: string, signal: AbortSignal): Promise<Snapshot[]> {
    try {
        const url = (process.env.NODE_ENV === 'development' ? 'http://localhost:8041/' : '') + `snapshots?start=${encodeURIComponent(start)}`;
        const response = await fetch(url, {signal});
        const snapshots: Snapshot[] = await response.json();

        return snapshots.map((snapshot) => ({
            ...snapshot,
            num_readers: snapshot.num_readers + 1,
            cluster_utilization: snapshot.max_cpu_utilization * (snapshot.num_readers + 1),
        }));
    } catch (error) {
        console.error('Error fetching data:', error);
        return [];
    }
}

function groupDataByTime(data: Snapshot[], timeInterval: number, mostRecentTimeInterval: number): Snapshot[] {
    const groupedData: Snapshot[] = [];
    let currentGroup: Snapshot | null = null;
    let numReadersSum = 0;
    let clusterUtilizationSum = 0;
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
        const interval = (!snapshot.future_value && mostRecentTimestamp - timestamp <= 60 * 60 * 1000)
            ? mostRecentTimeInterval
            : timeInterval;

        if (!currentGroup) {
            currentGroup = {...snapshot, timestamp: snapshot.timestamp, num_readers: 0, cluster_utilization: 0};
            numReadersSum = snapshot.num_readers;
            clusterUtilizationSum = snapshot.cluster_utilization;
            count = 1;
        } else if (timestamp - new Date(currentGroup.timestamp).getTime() >= interval) {
            (currentGroup as Snapshot).num_readers = numReadersSum / count;
            (currentGroup as Snapshot).cluster_utilization = clusterUtilizationSum / count;
            groupedData.push(currentGroup);
            currentGroup = {...snapshot, timestamp: snapshot.timestamp, num_readers: 0, cluster_utilization: 0};
            numReadersSum = snapshot.num_readers;
            clusterUtilizationSum = snapshot.cluster_utilization;
            count = 1;
        } else {
            numReadersSum += snapshot.num_readers;
            clusterUtilizationSum += snapshot.cluster_utilization;
            count++;
        }
    });

    if (currentGroup) {
        (currentGroup as Snapshot).num_readers = numReadersSum / count;
        (currentGroup as Snapshot).cluster_utilization = clusterUtilizationSum / count;
        groupedData.push(currentGroup);
    }

    return groupedData;
}


function App() {
    const [recent, setRecent] = useState<Snapshot[]>([]);
    const [cursor, setCursor] = useState<string | null>(null);
    const [fetchController, setFetchController] = useState<AbortController | null>(null);
    const [currentSnapshot, setCurrentSnapshot] = useState<Snapshot | null>(null);
    const [currentPrediction, setCurrentPrediction] = useState<Snapshot | null>(null);

    useEffect(() => {
        async function fetchRecent() {
            if (fetchController) {
                fetchController.abort(); // Abort ongoing fetch if exists
            }

            const controller = new AbortController();
            setFetchController(controller);

            try {
                const start: string = new Date().toISOString();
                const recentData = await fetchData(start, controller.signal);
                setRecent(recentData);
            } catch (error) {
                console.error('Error fetching data:', error)
            }
        }

        fetchRecent();
        const intervalId = setInterval(fetchRecent, 0.5 * 60 * 1000);

        return () => {
            clearInterval(intervalId); // Clear interval on unmount
            if (fetchController) {
                fetchController.abort(); // Abort ongoing fetch on unmount
            }
        };
    }, []);

    useEffect(() => {
        document.title = 'Predictive RDS Autoscaler ' + (recent.length && recent[0].cluster_name);
        for (let i = recent.length - 1; i >= 0; i--) {
            if (!recent[i].future_value) {
                setCurrentSnapshot(recent[i]);
                setCurrentPrediction(recent[i + 1]);
                break;
            }
        }
    }, [recent]);

    const handleMouseMove = (e: any) => {
        if (e && e.activeLabel) {
            setCursor(e.activeLabel);
        }
    };

    const handleMouseLeave = () => {
        setCursor(null);
    };

    const aggregatedData = groupDataByTime(recent, 5 * 60 * 1000, 1 * 60 * 1000);
    return (
        <ThemeProvider theme={theme}>
            <CssBaseline/>
            <Container>
                {aggregatedData.length === 0 &&
                    <Backdrop
                        sx={{color: '#fff', zIndex: (theme: Theme) => theme.zIndex.drawer + 1}}
                        open={true}
                    >
                        <CircularProgress color="inherit"/>
                    </Backdrop>}

                {aggregatedData.length > 0 &&
                    <Paper elevation={3} sx={{padding: 2, marginBottom: 3}}>
                        <Box sx={{width: '100%'}}>
                            <Typography variant="h4" gutterBottom>
                                Cluster Utilization {currentSnapshot && <Chip label={currentSnapshot.cluster_name} />}
                                {
                                    currentSnapshot && <><Box sx={{display: 'flex', alignItems: 'center', marginTop: '20px'}}>
                                        <Box sx={{width: '50%', mr: 1}}>
                                            <LinearProgress
                                                variant="buffer"
                                                color={currentSnapshot.max_cpu_utilization < 50 ? "success" : currentSnapshot.max_cpu_utilization < 75 ? "warning" : "error"}
                                                value={currentSnapshot.max_cpu_utilization}
                                                valueBuffer={currentPrediction?.max_cpu_utilization}
                                            />
                                        </Box>
                                        <Box sx={{minWidth: 35}}>
                                            <Typography color="text.secondary">
                                                {`${Math.round(currentSnapshot.max_cpu_utilization,)}% Current, `}
                                                {currentPrediction && `${Math.round(currentPrediction?.max_cpu_utilization,)}% Predicted`}
                                            </Typography>
                                        </Box>
                                    </Box>
                                        {currentPrediction &&
                                            <Box sx={{display: 'flex', alignItems: 'center'}}>
                                                <Box sx={{width: '50%', mr: 1}}>
                                                    <LinearProgress
                                                        variant="buffer"
                                                        value={currentSnapshot.num_readers / 5 * 100}
                                                        valueBuffer={currentPrediction.num_readers / 5 * 100}
                                                    />
                                                </Box>
                                                <Box sx={{minWidth: 35}}>
                                                    <Typography
                                                        color="text.secondary">
                                                        {currentSnapshot && ` ${Math.round(currentSnapshot?.num_readers,)} Current,`}
                                                        {currentPrediction && ` ${Math.round(currentPrediction?.num_readers,)} Predicted`}
                                                    </Typography>
                                                </Box>
                                            </Box>
                                        }
                                    </>}
                            </Typography>
                        </Box>
                        <ResponsiveContainer width="100%" height={300}>
                            <ComposedChart
                                style={{cursor: 'crosshair'}}
                                data={aggregatedData}
                                onMouseMove={handleMouseMove}
                                onMouseLeave={handleMouseLeave}

                            >
                                <XAxis dataKey="timestamp" tickFormatter={(timeStr) => ''}/>
                                <YAxis yAxisId="left"/>
                                <CartesianGrid strokeDasharray="2 1" opacity={0.3}/>
                                <Tooltip content={<CustomTooltip />} />
                                <Line
                                    type="monotone"
                                    dataKey="cluster_utilization"
                                    stroke="#82ca9d"
                                    strokeWidth={2}
                                    dot={false}
                                    yAxisId="left"
                                ></Line>
                                <Line
                                    type="monotone"
                                    dataKey="max_cpu_utilization"
                                    stroke="#fff"
                                    strokeWidth={2}
                                    dot={false}
                                    yAxisId="left"
                                />
                            </ComposedChart>
                        </ResponsiveContainer>
                        <ResponsiveContainer width="100%" height={120}>
                            <ComposedChart
                                style={{cursor: 'crosshair'}}
                                data={aggregatedData}
                                onMouseMove={handleMouseMove}
                                onMouseLeave={handleMouseLeave}

                            >
                                <XAxis dataKey="timestamp" tickFormatter={(timeStr) => timeStr.slice(11, 16)}/>
                                <YAxis/>
                                <Bar dataKey="num_readers"
                                     barSize={20}
                                     style={{transition: 'fill 0.3s'}}
                                >
                                    {aggregatedData.map((snapshot: Snapshot, index) => (
                                        <Cell key={index}
                                              fill={snapshot.future_value ? 'rgb(171,35,35)' : snapshot.predicted_value ? 'rgb(0,162,191)' : 'rgb(140,140,140)'}
                                        />
                                    ))}
                                </Bar>
                            </ComposedChart>
                        </ResponsiveContainer>
                    </Paper>
                }

                {
                    cursor && (
                        <Paper elevation={3} sx={{padding: 2, marginTop: 3}}>
                            <Typography variant="h6">Cursor Details</Typography>
                            {recent.some((item) => item.timestamp === cursor) ? (
                                <>
                                    {recent.map((item) => {
                                        if (item.timestamp === cursor) {
                                            const {
                                                num_readers,
                                                cluster_utilization,
                                                max_cpu_utilization,
                                                predicted_value,
                                                future_value
                                            } = item;
                                            return (
                                                <div key={item.timestamp}>
                                                    <Typography>Timestamp: {cursor}</Typography>
                                                    <Typography>Cluster Size: {num_readers}</Typography>
                                                    <Typography>Cluster
                                                        Utilization: {cluster_utilization.toPrecision(4)}%</Typography>
                                                    <Typography>Max
                                                        Utilization: {max_cpu_utilization.toPrecision(4)}%</Typography>
                                                    <Typography>
                                                        {future_value && "Forecasted value"}
                                                        {predicted_value && "Historic value used for scaling"}
                                                    </Typography>
                                                </div>
                                            );
                                        }
                                        return null;
                                    })}
                                </>
                            ) : (
                                <Typography>No details available for this timestamp.</Typography>
                            )}
                        </Paper>
                    )
                }
            </Container>
        </ThemeProvider>
    )
        ;
}

export default App;

const CustomTooltip = ({ active, payload, label }: any) => {
    if (active && payload && payload.length) {
        const item = payload[0].payload;
        const {
            num_readers,
            cluster_utilization,
            max_cpu_utilization,
            predicted_value,
            future_value
        } = item;

        return (
            <Box>
                {/*<Typography variant="body2">Timestamp: {label}</Typography>
                <Typography variant="body2">Cluster Size: {num_readers}</Typography>
                <Typography variant="body2">Cluster Utilization: {cluster_utilization.toPrecision(4)}%</Typography>
                <Typography variant="body2">Max Utilization: {max_cpu_utilization.toPrecision(4)}%</Typography>
                <Typography variant="body2">
                    {future_value && "Forecasted value"}
                    {predicted_value && "Historic value used for scaling"}
                </Typography>*/}
            </Box>
        );
    }

    return null;
};
