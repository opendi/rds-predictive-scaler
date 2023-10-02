import React, { useState } from 'react';
import { Modal, Box, Typography, Slider, Button } from '@mui/material';
import {Config} from "../types/Config";

interface SettingsModalProps {
    open: boolean;
    onClose: () => void;
    onSave: (settings: Config) => void;
}

const SettingsModal: React.FC<SettingsModalProps> = ({ open, onClose, onSave }) => {
    const [settings, setSettings] = useState<Config>({
        MaxInstances: 0,
        MinInstances: 0,
        TargetCpuUtil: 0,
        PlanAheadTime: 0,
        ScaleOutCooldown: 0,
        ScaleInCooldown: 0,
        ScaleInStep: 0,
        ScaleOutStep: 0,
    });

    const handleChange = (key: keyof Config, value: number) => {
        setSettings({ ...settings, [key]: value });
    };

    const handleSubmit = () => {
        // Call the onSave function with the current settings
        onSave(settings);

        // Close the modal
        onClose();
    };

    return (
        <Modal open={open} onClose={onClose}>
            <Box
                sx={{
                    position: 'absolute',
                    top: '50%',
                    left: '50%',
                    transform: 'translate(-50%, -50%)',
                    bgcolor: 'white',
                    boxShadow: 24,
                    p: 4,
                    width: 400,
                }}
            >
                <Typography variant="h5" gutterBottom>
                    Settings
                </Typography>
                <div>
                    <Typography id="cluster-size-slider" gutterBottom>
                        Cluster size
                    </Typography>
                    <Slider
                        valueLabelDisplay="auto"
                        valueLabelFormat={(value) => value.toString()}
                        step={1}
                        min={0}
                        max={10}
                        onChange={(e, value) => handleChange('MinInstances', value as number)}
                        value={[settings.MinInstances, settings.MaxInstances]}
                    />
                </div>
                <div>
                    <Typography id="target-utilization-slider" gutterBottom>
                        Target Utilization
                    </Typography>
                    <Slider
                        valueLabelDisplay="auto"
                        step={1}
                        min={0}
                        max={100}
                        onChange={(e, value) => handleChange('TargetCpuUtil', value as number)}
                        value={settings.TargetCpuUtil}
                    />
                </div>
                <Button variant="contained" color="primary" onClick={handleSubmit}>
                    Save
                </Button>
                <Button variant="contained" color="secondary" onClick={onClose}>
                    Cancel
                </Button>
            </Box>
        </Modal>
    );
};

export default SettingsModal;
