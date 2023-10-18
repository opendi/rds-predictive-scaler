import {Box, Card, LinearProgress, Typography} from "@mui/material";
import Snapshot from "../types/Snapshot";
import {useEffect} from "react";

const CurrentLoad = (props: { currentSnapshot: Snapshot, currentPrediction: Snapshot | null }) => {
    const {currentSnapshot, currentPrediction} = props;
    const currentColor = currentSnapshot.max_cpu_utilization < 50 ? "success" : currentSnapshot.max_cpu_utilization < 75 ? "warning" : "error";

    const fullBars = Math.min(Math.floor(currentSnapshot.max_cpu_utilization / 10), 10);
    const emptyBars = Math.max(0, 10 - fullBars);

    // Generate the progress bar string
    const progressBar = '|'.repeat(fullBars) + 'ˌ'.repeat(emptyBars);

    useEffect(() => {
        document.title = `${Math.floor(currentSnapshot.max_cpu_utilization)}% ${progressBar} RDS pScalr`;
    }, [progressBar, currentSnapshot]);


    return (
        <Card sx={{width: '30%', mr: 1, display: 'flex', flexWrap: 'wrap', padding: '10px'}}>
            <Box sx={{display: 'flex', alignItems: 'flex-start', flexDirection: 'column', width: 'calc(50%-10px)'}}>
                <Typography variant={"h6"}>Load</Typography>
                <Typography variant="h2" component="div" color={"success"}>
                    {currentSnapshot.max_cpu_utilization.toPrecision(3)}%
                </Typography>
            </Box>
            {currentPrediction &&
                <Box sx={{display: 'flex', alignItems: 'flex-end', width: 'calc(50%-10px)'}}>
                    <Typography variant="h4" component="div" color={"primary"}>
                        {currentPrediction.max_cpu_utilization.toPrecision(3)}%
                    </Typography>
                </Box>}
            <LinearProgress
                sx={{width: '100%', alignItems: 'flex-end'}}
                variant="buffer"
                color={currentColor}
                value={currentSnapshot.max_cpu_utilization}
                valueBuffer={currentPrediction?.max_cpu_utilization || 0}
            />
        </Card>
    )
}
export default CurrentLoad;