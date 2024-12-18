import subprocess
import time
import logging
import os
from flask import Flask, Response
from prometheus_client import Gauge, generate_latest, CollectorRegistry

# Set up logging
logging.basicConfig(level=logging.INFO)

# Initialize Flask app
app = Flask(__name__)

# Create a registry for Prometheus metrics
registry = CollectorRegistry()

# Define a gauge metric
gauge = Gauge(
    'custom_metrics',
    'Custom metrics from script execution',
    ['component', 'process_name', 'application_name', 'env', 'domain_name', 'mon_type'],
    registry=registry,
)

def reap_children():
    """Reap any child processes that have exited."""
    while True:
        try:
            # Wait for any child process without blocking (non-blocking)
            pid, _ = os.waitpid(-1, os.WNOHANG)
            if pid == 0:  # No child process has exited
                break
        except ChildProcessError:
            # No child processes exist
            break

def execute_command(script):
    """Execute the shell command and return parsed metrics."""
    try:
        # Run the shell command and wait for it to complete
        output = subprocess.check_output(script, shell=True, text=True).strip()
        metrics = []
        
        # Process each line of output
        for line in output.split('\n'):
            fields = line.split(',')
            if len(fields) != 7:
                logging.error(f"Invalid output format: {line}")
                continue
            
            # Parse the metric value
            try:
                value = float(fields[6].strip())
                metrics.append({
                    'component': fields[0].strip(),
                    'process_name': fields[1].strip(),
                    'application_name': fields[2].strip(),
                    'env': fields[3].strip(),
                    'domain_name': fields[4].strip(),
                    'mon_type': fields[5].strip(),
                    'value': value,
                })
            except ValueError:
                logging.error(f"Invalid metric value: {fields[6]}")
        
        return metrics
    
    except subprocess.CalledProcessError as e:
        logging.error(f"Error executing command: {e}")
        return []

def update_metrics(script):
    """Update Prometheus metrics periodically."""
    while True:
        metrics = execute_command(script)
        
        # Clear previous values and update new ones
        gauge.clear()
        
        for metric in metrics:
            gauge.labels(
                component=metric['component'],
                process_name=metric['process_name'],
                application_name=metric['application_name'],
                env=metric['env'],
                domain_name=metric['domain_name'],
                mon_type=metric['mon_type']
            ).set(metric['value'])
        
        logging.info("Metrics updated successfully.")
        
        # Reap any defunct children before sleeping
        reap_children()
        
        time.sleep(5)  # Adjust this interval as needed

@app.route('/metrics')
def metrics_endpoint():
    """Expose metrics to Prometheus."""
    return Response(generate_latest(registry), mimetype='text/plain')

if __name__ == '__main__':
    import argparse
    
    parser = argparse.ArgumentParser(description='Custom Metrics Exporter')
    parser.add_argument('-script', required=True, help='Path to the shell script to execute')
    parser.add_argument('-port', default='8000', help='Port to run the HTTP server on')
    
    args = parser.parse_args()
    
    # Start updating metrics in a separate thread or process if needed.
    import threading
    threading.Thread(target=update_metrics, args=(args.script,), daemon=True).start()
    
    # Start the Flask app
    app.run(host='0.0.0.0', port=int(args.port))
