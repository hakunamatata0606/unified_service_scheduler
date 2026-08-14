const form = document.querySelector('#booking-form');
const loginForm = document.querySelector('#login-form');
const loginCard = document.querySelector('#login-card');
const bookingCard = document.querySelector('#booking-card');
const resultCard = document.querySelector('#result-card');
const appointmentsCard = document.querySelector('#appointments-card');
const appointmentRows = document.querySelector('#my-appointments');
const output = document.querySelector('#output');
const status = document.querySelector('#status');
const loginStatus = document.querySelector('#login-status');
let pendingAttempt;

function appointmentCell(value) {
  const cell = document.createElement('td');
  cell.textContent = value;
  return cell;
}

async function loadAppointments() {
  const response = await fetch('/api/v1/my/appointments');
  const appointments = await response.json();
  if (!response.ok) throw new Error(appointments.error || 'Could not load appointments');
  appointmentRows.replaceChildren();
  if (!appointments.length) {
    appointmentRows.innerHTML = '<tr><td colspan="7" class="muted">You have no registered appointments yet.</td></tr>';
    return;
  }
  appointments.forEach((appointment) => {
    const row = document.createElement('tr');
    row.append(
      appointmentCell(`${appointment.vehicle_make} ${appointment.vehicle_model}`),
      appointmentCell(appointment.service_type_name),
      appointmentCell(appointment.dealership_name),
      appointmentCell(appointment.technician_name),
      appointmentCell(appointment.service_bay_name),
      appointmentCell(`${new Date(appointment.start_time_utc).toLocaleString()} - ${new Date(appointment.end_time_utc).toLocaleString()}`),
      appointmentCell(appointment.status),
    );
    appointmentRows.append(row);
  });
}

async function loadOptions() {
  const [vehicleResponse, serviceResponse, dealershipResponse] = await Promise.all([
    fetch('/api/v1/vehicles'),
    fetch('/api/v1/service-types'),
    fetch('/api/v1/dealerships'),
  ]);
  const vehicles = await vehicleResponse.json();
  const services = await serviceResponse.json();
  const dealerships = await dealershipResponse.json();
  if (!vehicleResponse.ok || !serviceResponse.ok || !dealershipResponse.ok) {
    throw new Error(vehicles.error || services.error || dealerships.error || 'Could not load options');
  }

  const vehicleSelect = form.elements.vehicleId;
  const serviceSelect = form.elements.serviceTypeId;
  const dealershipSelect = form.elements.dealershipId;
  vehicleSelect.replaceChildren(...vehicles.map((vehicle) => new Option(`${vehicle.make} ${vehicle.model} - ${vehicle.vin}`, vehicle.id)));
  serviceSelect.replaceChildren(...services.map((service) => new Option(`${service.name} - ${service.duration_minutes} minutes`, service.id)));
  dealershipSelect.replaceChildren(...dealerships.map((dealership) => new Option(`${dealership.name} (${dealership.timezone})`, dealership.id)));
  vehicleSelect.disabled = false;
  serviceSelect.disabled = false;
  dealershipSelect.disabled = false;
}

async function checkSession() {
  const response = await fetch('/api/auth/me');
  if (!response.ok) return false;
  await Promise.all([loadOptions(), loadAppointments()]);
  loginCard.hidden = true;
  bookingCard.hidden = false;
  resultCard.hidden = false;
  appointmentsCard.hidden = false;
  return true;
}

loginForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  loginStatus.textContent = 'Signing in...';
  const response = await fetch('/api/auth/login', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({email: loginForm.elements.email.value, password: loginForm.elements.password.value}),
  });
  const body = await response.json();
  if (!response.ok) {
    loginStatus.textContent = body.error || 'Login failed';
    return;
  }
  loginStatus.textContent = '';
  await checkSession();
});

form.addEventListener('submit', async (event) => {
  event.preventDefault();
  const start = new Date(form.elements.requestedStart.value);
  const payload = {
    vehicleId: form.elements.vehicleId.value,
    dealershipId: form.elements.dealershipId.value,
    serviceTypeId: form.elements.serviceTypeId.value,
    requestedStart: start.toISOString(),
  };
  const serialized = JSON.stringify(payload);
  if (!pendingAttempt || pendingAttempt.payload !== serialized) {
    pendingAttempt = {
      payload: serialized,
      key: crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`,
    };
  }

  status.textContent = 'Checking live availability...';
  try {
    const query = new URLSearchParams({
      dealershipId: payload.dealershipId,
      serviceTypeId: payload.serviceTypeId,
      start: payload.requestedStart,
    });
    const availabilityResponse = await fetch(`/api/v1/availability?${query}`);
    const availability = await availabilityResponse.json();
    if (!availabilityResponse.ok) throw new Error(availability.error || 'Could not check availability');
    if (!availability.available) {
      status.textContent = 'Not available';
      output.textContent = 'No qualified technician and compatible service bay are both free for the full service duration. Please choose another time or dealership.';
      return;
    }

    status.textContent = 'Booking...';
    const response = await fetch('/api/v1/appointments', {
      method: 'POST',
      headers: {'Content-Type': 'application/json', 'Idempotency-Key': pendingAttempt.key},
      body: serialized,
    });
    const body = await response.json();
    status.textContent = `${response.status} ${response.statusText}`;
    output.textContent = response.ok
      ? `Appointment confirmed\n\nReference: ${body.id}\nStart: ${new Date(body.start_time_utc).toLocaleString()}\nEnd: ${new Date(body.end_time_utc).toLocaleString()}\nStatus: ${body.status}`
      : (body.error || 'Booking failed');
    if (response.ok) await loadAppointments();
  } catch (error) {
    status.textContent = 'Request failed';
    output.textContent = error.message;
  }
});

document.querySelector('#refresh-appointments').addEventListener('click', () => {
  loadAppointments().catch((error) => {
    appointmentRows.innerHTML = `<tr><td colspan="7">${error.message}</td></tr>`;
  });
});

checkSession().catch((error) => { loginStatus.textContent = error.message; });
