const rows = document.querySelector('#appointments');
const status = document.querySelector('#status');
const userLabel = document.querySelector('#user-label');
const dealershipSelect = document.querySelector('#admin-dealership');
const serviceSelect = document.querySelector('#technician-filter').elements.serviceTypeId;

function format(value) {
  return value ? new Date(value).toLocaleString() : '-';
}

function cell(value) {
  const td = document.createElement('td');
  td.textContent = value;
  return td;
}

async function loadAppointments() {
  if (!dealershipSelect.value) return;
  status.textContent = 'Loading...';
  const params = new URLSearchParams({
    dealershipId: dealershipSelect.value,
    from: new Date('2020-01-01T00:00:00Z').toISOString(),
    to: new Date('2035-01-01T00:00:00Z').toISOString(),
  });

  try {
    const response = await fetch(`/api/v1/admin/appointments?${params}`);
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || response.statusText);
    rows.replaceChildren();
    if (!body.length) {
      rows.innerHTML = '<tr><td colspan="9" class="muted">No appointments yet.</td></tr>';
    } else {
      body.forEach((appointment) => {
        const row = document.createElement('tr');
        row.append(
          cell(appointment.id),
          cell(`${appointment.vehicle_make} ${appointment.vehicle_model} (${appointment.customer_name}, ${appointment.user_email})`),
          cell(appointment.service_type_name),
          cell(appointment.technician_name),
          cell(appointment.service_bay_name),
          cell(appointment.dealership_name),
          cell(format(appointment.start_time_utc)),
          cell(format(appointment.end_time_utc)),
          cell(appointment.status),
        );
        rows.append(row);
      });
    }
    status.textContent = `${body.length} appointment${body.length === 1 ? '' : 's'}`;
  } catch (error) {
    status.textContent = 'Failed to load';
    rows.innerHTML = `<tr><td colspan="9">${error.message}</td></tr>`;
  }
}

async function loadReferenceData() {
  const [dealershipResponse, serviceResponse] = await Promise.all([
    fetch('/api/v1/dealerships'),
    fetch('/api/v1/service-types'),
  ]);
  const dealerships = await dealershipResponse.json();
  const services = await serviceResponse.json();
  if (!dealershipResponse.ok || !serviceResponse.ok) throw new Error(dealerships.error || services.error || 'Could not load filters');
  dealershipSelect.replaceChildren(...dealerships.map((item) => new Option(item.name, item.id)));
  serviceSelect.replaceChildren(...services.map((item) => new Option(`${item.name} - ${item.duration_minutes} minutes`, item.id)));
  dealershipSelect.disabled = false;
  await loadAppointments();
}

fetch('/api/auth/me').then(async (response) => {
  if (!response.ok) { userLabel.textContent = 'Not signed in'; return; }
  const user = await response.json();
  userLabel.textContent = user.email;
}).catch(() => { userLabel.textContent = 'Not signed in'; });

document.querySelector('#logout').addEventListener('click', async () => {
  await fetch('/api/auth/logout', { method: 'POST' });
  window.location.href = '/';
});
document.querySelector('#refresh').addEventListener('click', loadAppointments);
dealershipSelect.addEventListener('change', loadAppointments);
loadReferenceData().catch((error) => { status.textContent = error.message; });
