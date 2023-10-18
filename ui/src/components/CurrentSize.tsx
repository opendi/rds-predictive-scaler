import {Box, Card, LinearProgress, Typography} from "@mui/material";
import Snapshot from "../types/Snapshot";

const CurrentSize = (props: { currentSnapshot: Snapshot, currentPrediction: Snapshot | null }) => {
    const {currentSnapshot, currentPrediction} = props;
    return (
        <Card sx={{width: '30%', mr: 1, display: 'flex', flexWrap: 'wrap', padding: '10px'}}>
            <Box sx={{display: 'flex', alignItems: 'flex-start', flexDirection: 'column', width: 'calc(50%-10px)'}}>
                <Typography variant={"h6"}>Size</Typography>
                <Typography variant="h2" component="div" >
                    {currentSnapshot.num_readers} <Typography variant={"h6"} component={"span"}>Readers</Typography>
                </Typography>
            </Box>
            {currentPrediction &&
                <Box sx={{display: 'flex', alignItems: 'flex-end', width: 'calc(50%-10px)'}}>
                    <Typography variant="h4" component="div" color={"primary"}>
                        {currentPrediction?.num_readers}
                    </Typography>
                </Box>}
            <LinearProgress
                sx={{width: '100%', alignItems: 'flex-end'}}
                variant="buffer"
                value={(currentSnapshot.num_readers) / 5 * 100}
                valueBuffer={(currentPrediction?.num_readers || 0) / 5 * 100}
            />
        </Card>
    )
}
export default CurrentSize;