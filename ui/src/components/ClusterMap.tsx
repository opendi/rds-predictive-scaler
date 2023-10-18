import React, {useState} from 'react';
import {Avatar, Box, Card, CardContent, CardHeader, Divider, Grid, Typography} from '@mui/material';
import {green, orange, red} from '@mui/material/colors';
import {MultipleStop} from "@mui/icons-material";

interface ServerStatusMapping {
    [status: string]: { bgcolor: string; color: string };
}

export interface InstanceStatus {
    name: string;
    cpu_utilization: number;
    status: string;
    is_writer: boolean;
}

interface ClusterMapProps {
    clusterStatus: InstanceStatus[];
}

const serverStatusMapping: ServerStatusMapping = {
    'available': {bgcolor: green[100], color: 'black'},
    'backing-up': {bgcolor: orange[100], color: 'black'},
    'configuring-enhanced-monitoring': {bgcolor: orange[100], color: 'black'},
    'configuring-iam-database-auth': {bgcolor: orange[100], color: 'black'},
    'configuring-log-exports': {bgcolor: orange[100], color: 'black'},
    'converting-to-vpc': {bgcolor: orange[100], color: 'black'},
    'creating': {bgcolor: orange[100], color: 'black'},
    'delete-precheck': {bgcolor: green[100], color: 'black'},
    'deleting': {bgcolor: red[500], color: 'white'},
    'failed': {bgcolor: red[500], color: 'white'},
    'inaccessible-encryption-credentials': {bgcolor: red[500], color: 'white'},
    'inaccessible-encryption-credentials-recoverable': {bgcolor: red[500], color: 'white'},
    'incompatible-network': {bgcolor: red[500], color: 'white'},
    'incompatible-option-group': {bgcolor: red[500], color: 'white'},
    'incompatible-parameters': {bgcolor: red[500], color: 'white'},
    'incompatible-restore': {bgcolor: red[500], color: 'white'},
    'insufficient-capacity': {bgcolor: red[500], color: 'white'},
    'maintenance': {bgcolor: orange[100], color: 'black'},
    'modifying': {bgcolor: orange[100], color: 'black'},
    'moving-to-vpc': {bgcolor: orange[100], color: 'black'},
    'rebooting': {bgcolor: orange[100], color: 'black'},
    'resetting-master-credentials': {bgcolor: orange[100], color: 'black'},
    'renaming': {bgcolor: orange[100], color: 'black'},
    'restore-error': {bgcolor: red[500], color: 'white'},
    'starting': {bgcolor: orange[100], color: 'black'},
    'stopped': {bgcolor: red[500], color: 'white'},
    'stopping': {bgcolor: red[500], color: 'white'},
    'storage-full': {bgcolor: red[500], color: 'white'},
    'storage-optimization': {bgcolor: orange[100], color: 'black'},
    'upgrading': {bgcolor: orange[100], color: 'black'},
};


const ClusterMap: React.FC<ClusterMapProps> = ({clusterStatus}) => {
    const [selectedServer, setSelectedInstance] = useState<InstanceStatus | null>(null);

    return (
        <Box>
            <Box>
                <Typography gutterBottom variant="h4" component="div">
                    <MultipleStop fontSize={"large"} /> Cluster Members
                </Typography>
            </Box>

            <Divider variant={"middle"}/>

            <Box sx={{ m: 2 }}>
                <Grid container spacing={2}>
                    {clusterStatus
                        .slice()
                        .sort((a, b) => {
                            return a.is_writer ? -1 : a.status.localeCompare(b.status)
                        }) // sort by writer first, then by status
                        .map((instanceStatus, index) => {
                            const statusConfig = serverStatusMapping[instanceStatus.status] || {
                                bgcolor: red[500],
                                color: 'white'
                            };
                            return (
                                <Grid item key={index} xs={3}>
                                    <Card
                                        className={`server-card ${instanceStatus.status}`}
                                        onClick={() => setSelectedInstance(instanceStatus)}
                                        sx={{cursor: 'pointer'}}
                                    >
                                        <CardHeader
                                            avatar={
                                                <Avatar sx={{bgcolor: statusConfig.bgcolor, color: statusConfig.color}}
                                                        aria-label="recipe">
                                                    {instanceStatus.is_writer ? 'W' : 'R'}
                                                </Avatar>
                                            }
                                            title={instanceStatus.name}
                                            subheader={instanceStatus.status}
                                        />
                                        <CardContent>
                                            <Typography variant="body1">CPU
                                                Usage: {Math.round(instanceStatus.cpu_utilization)}%</Typography>
                                        </CardContent>
                                    </Card>
                                </Grid>
                            );
                        })}
                </Grid>
            </Box>

            {
                selectedServer && (
                    <div className="details-bar">
                        {/* Render details and actions for the selected server here */}
                        {/* You can use another component to display detailed information */}
                    </div>
                )
            }
        </Box>);
};

export default ClusterMap;
