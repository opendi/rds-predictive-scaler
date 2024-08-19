import {Box, LinearProgress, Typography} from "@mui/material";
import {useEffect} from "react";

const CurrentLoad = (props: { status: ClusterStatus, prediction: ClusterStatus|null }) => {
    const {status, prediction} = props;
    const currentColor = status.average_cpu_utilization < 50 ? "success" : status.average_cpu_utilization < 75 ? "warning" : "error";

    const fullBars = Math.min(Math.floor(status.average_cpu_utilization / 10), 10);
    const emptyBars = Math.max(0, 10 - fullBars);

    // Generate the progress bar string
    const progressBar = '|'.repeat(fullBars) + 'ˌ'.repeat(emptyBars);

    useEffect(() => {
        document.title = `${Math.floor(status.average_cpu_utilization)}% ${progressBar} RDS pScalr`;
    }, [progressBar, status]);


    return (
        <>
            <Box sx={{display: 'flex', alignItems: 'flex-start', flexDirection: 'column', width: 'calc(50%-10px)'}}>
                <Typography variant={"h6"}>Load</Typography>
                <Typography variant="h2" component="div" color={"success"}>
                    {status.average_cpu_utilization.toPrecision(3)}%
                </Typography>
            </Box>
            {prediction &&
                <Box sx={{display: 'flex', alignItems: 'flex-end', width: 'calc(50%-10px)'}}>
                    <Typography variant="h4" component="div" color={"primary"}>
                        {prediction.average_cpu_utilization.toPrecision(3)}%
                    </Typography>
                </Box>}
            <LinearProgress
                sx={{width: '100%', alignItems: 'flex-end'}}
                variant="buffer"
                color={currentColor}
                value={status.average_cpu_utilization}
                valueBuffer={prediction?.average_cpu_utilization || 0}
            />
        </>
    )
}
export default CurrentLoad;